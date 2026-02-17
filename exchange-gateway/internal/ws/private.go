// Package ws Private WebSocket server with API key signature auth.
package ws

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/exchange/gateway/internal/middleware"
	"github.com/gorilla/websocket"
)

const (
	queryAPIKey    = "apiKey"
	queryTimestamp = "timestamp"
	queryNonce     = "nonce"
	querySignature = "signature"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	activityTimeoutNanos int64 = int64(60 * time.Second)
	pingIntervalNanos    int64 = int64(30 * time.Second)
	writeWaitNanos       int64 = int64(10 * time.Second)
	authTimeoutNanos     int64 = int64(5 * time.Second)
)

// PrivateHandler handles /ws/private connections.
func PrivateHandler(hub *Hub, authCfg *middleware.AuthConfig, allowedOrigins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		localUpgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return allowOrigin(r, allowedOrigins)
			},
		}

		conn, err := localUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("private ws upgrade error: %v", err)
			return
		}

		userID, err := authenticateRequest(r, authCfg)
		if err != nil {
			closeWithCode(conn, 4001, "unauthorized")
			return
		}

		client, err := hub.Subscribe(userID, conn)
		if err != nil {
			closeMsg := websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "too many connections")
			_ = conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(getWriteWait()))
			conn.Close()
			return
		}

		go writePump(client, userID, hub)
		go readPump(client, userID, hub)
	}
}

func allowOrigin(r *http.Request, allowed []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients usually don't send Origin.
		return true
	}
	for _, o := range allowed {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

func readPump(client *Client, userID int64, hub *Hub) {
	conn := client.conn
	defer func() {
		hub.Unsubscribe(userID, client)
		conn.Close()
	}()

	conn.SetReadLimit(4096)
	conn.SetReadDeadline(time.Now().Add(getActivityTimeout()))
	conn.SetPongHandler(func(string) error {
		client.touch()
		conn.SetReadDeadline(time.Now().Add(getActivityTimeout()))
		return nil
	})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
		client.touch()
		conn.SetReadDeadline(time.Now().Add(getActivityTimeout()))
	}
}

func writePump(client *Client, userID int64, hub *Hub) {
	ticker := time.NewTicker(getPingInterval())
	defer func() {
		ticker.Stop()
		hub.Unsubscribe(userID, client)
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			client.conn.SetWriteDeadline(time.Now().Add(getWriteWait()))
			if !ok {
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			client.conn.SetWriteDeadline(time.Now().Add(getWriteWait()))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func authenticateRequest(r *http.Request, cfg *middleware.AuthConfig) (int64, error) {
	if cfg == nil || cfg.VerifySignature == nil {
		return 0, fmt.Errorf("auth config missing")
	}

	query := r.URL.Query()
	apiKey := query.Get(queryAPIKey)
	timestampStr := query.Get(queryTimestamp)
	nonce := query.Get(queryNonce)
	signature := query.Get(querySignature)

	if apiKey == "" || timestampStr == "" || nonce == "" || signature == "" {
		return 0, fmt.Errorf("missing auth params")
	}

	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp")
	}

	window := cfg.TimeWindow
	if window == 0 {
		window = 30 * time.Second
	}
	now := time.Now().UnixMilli()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > window.Milliseconds() {
		return 0, fmt.Errorf("timestamp expired")
	}

	ctx, cancel := context.WithTimeout(r.Context(), getAuthTimeout())
	defer cancel()

	resultCh := make(chan struct {
		userID      int64
		permissions int
		err         error
	}, 1)
	go func() {
		cleanQuery := cloneQueryWithoutSignature(query)
		userID, permissions, err := cfg.VerifySignature(ctx, &middleware.VerifySignatureRequest{
			APIKey:    apiKey,
			Timestamp: timestamp,
			Nonce:     nonce,
			Signature: signature,
			Method:    r.Method,
			Path:      r.URL.Path,
			Query:     cleanQuery,
			BodyHash:  "",
			ClientIP:  middleware.ClientIPFromRequest(r),
		})
		resultCh <- struct {
			userID      int64
			permissions int
			err         error
		}{userID: userID, permissions: permissions, err: err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			return 0, fmt.Errorf("invalid signature")
		}
		if (res.permissions & middleware.PermRead) == 0 {
			return 0, fmt.Errorf("permission denied")
		}
		return res.userID, nil
	case <-ctx.Done():
		return 0, fmt.Errorf("auth timeout")
	}
}

func cloneQueryWithoutSignature(query map[string][]string) map[string][]string {
	if len(query) == 0 {
		return map[string][]string{}
	}

	cloned := make(map[string][]string, len(query))
	for k, v := range query {
		if strings.EqualFold(k, querySignature) {
			continue
		}
		copied := make([]string, len(v))
		copy(copied, v)
		cloned[k] = copied
	}
	return cloned
}

func buildCanonicalString(timestamp int64, nonce, method, path string, query map[string][]string) string {
	parts := []string{
		fmt.Sprintf("%d", timestamp),
		nonce,
		strings.ToUpper(method),
		path,
		canonicalQuery(query),
	}
	return strings.Join(parts, "\n")
}

func canonicalQuery(query map[string][]string) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		values := query[k]
		sort.Strings(values)
		for _, v := range values {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(pairs, "&")
}

func sign(secret, data string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func closeWithCode(conn *websocket.Conn, code int, message string) {
	conn.SetWriteDeadline(time.Now().Add(getWriteWait()))
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, message))
	conn.Close()
}

func getActivityTimeout() time.Duration {
	return time.Duration(atomic.LoadInt64(&activityTimeoutNanos))
}

func setActivityTimeoutForTest(d time.Duration) {
	atomic.StoreInt64(&activityTimeoutNanos, int64(d))
}

func getPingInterval() time.Duration {
	return time.Duration(atomic.LoadInt64(&pingIntervalNanos))
}

func setPingIntervalForTest(d time.Duration) {
	atomic.StoreInt64(&pingIntervalNanos, int64(d))
}

func getWriteWait() time.Duration {
	return time.Duration(atomic.LoadInt64(&writeWaitNanos))
}

func setWriteWaitForTest(d time.Duration) {
	atomic.StoreInt64(&writeWaitNanos, int64(d))
}

func getAuthTimeout() time.Duration {
	return time.Duration(atomic.LoadInt64(&authTimeoutNanos))
}

func setAuthTimeoutForTest(d time.Duration) {
	atomic.StoreInt64(&authTimeoutNanos, int64(d))
}

// PrivateMessage is sent to clients.
type PrivateMessage struct {
	Channel string          `json:"channel"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"data"`
}
