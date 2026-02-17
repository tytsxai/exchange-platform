package ws

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/exchange/gateway/internal/middleware"
	"github.com/gorilla/websocket"
)

func TestAuthPrivateHandlerPingAndRead(t *testing.T) {
	origPing := getPingInterval()
	origTimeout := getActivityTimeout()
	origWrite := getWriteWait()
	setPingIntervalForTest(10 * time.Millisecond)
	setActivityTimeoutForTest(50 * time.Millisecond)
	setWriteWaitForTest(50 * time.Millisecond)
	defer func() {
		setPingIntervalForTest(origPing)
		setActivityTimeoutForTest(origTimeout)
		setWriteWaitForTest(origWrite)
	}()

	hub := NewHub()
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			if req.APIKey == "ping-key" && req.Signature == "ping-secret" {
				return 1, middleware.PermRead, nil
			}
			return 0, 0, errInvalidAPIKey
		},
	}

	server := httptest.NewServer(PrivateHandler(hub, authCfg, []string{"*"}))
	defer server.Close()

	timestamp := time.Now().UnixMilli()
	values := url.Values{}
	values.Set(queryAPIKey, "ping-key")
	values.Set(queryTimestamp, strconv.FormatInt(timestamp, 10))
	values.Set(queryNonce, "nonce")
	values.Set(querySignature, "ping-secret")

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/private?" + values.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("write message: %v", err)
	}

	time.Sleep(40 * time.Millisecond)
}
