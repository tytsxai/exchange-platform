package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/exchange/gateway/internal/middleware"
	"github.com/gorilla/websocket"
)

func TestAuthPrivateHandlerAuthAndBroadcast(t *testing.T) {
	hub := NewHub()
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			if req.APIKey == "test-api-key" && req.Signature == "sig-test" {
				return 123, 0, nil
			}
			return 0, 0, errInvalidAPIKey
		},
	}

	server := httptest.NewServer(PrivateHandler(hub, authCfg))
	defer server.Close()

	timestamp := time.Now().UnixMilli()
	values := url.Values{}
	values.Set(queryAPIKey, "test-api-key")
	values.Set(queryTimestamp, strconv.FormatInt(timestamp, 10))
	values.Set(queryNonce, "nonce")
	values.Set(querySignature, "sig-test")

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/private?" + values.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool {
		return hub.ConnectionCount() == 1
	})

	payload := PrivateMessage{
		Channel: "order",
		Data:    json.RawMessage(`{"id":1}`),
	}
	message, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	hub.Broadcast(123, message)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != string(message) {
		t.Fatalf("message = %s, want %s", string(got), string(message))
	}
}

func TestAuthPrivateHandlerAuthFailure(t *testing.T) {
	hub := NewHub()
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			if req.APIKey == "test-api-key" && req.Signature == "sig-test" {
				return 123, 0, nil
			}
			return 0, 0, errInvalidAPIKey
		},
	}

	server := httptest.NewServer(PrivateHandler(hub, authCfg))
	defer server.Close()

	values := url.Values{}
	values.Set(queryAPIKey, "test-api-key")
	values.Set(queryTimestamp, strconv.FormatInt(time.Now().UnixMilli(), 10))
	values.Set(queryNonce, "nonce")
	values.Set(querySignature, "bad-signature")

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/private?" + values.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatal("expected close error")
	}
	closeErr, ok := err.(*websocket.CloseError)
	if !ok || closeErr.Code != 4001 {
		t.Fatalf("expected close code 4001, got %v", err)
	}
}

func TestAuthPrivateHandlerUnauthorizedResponse(t *testing.T) {
	hub := NewHub()
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			return 1, 0, nil
		},
	}
	server := httptest.NewServer(PrivateHandler(hub, authCfg))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/private"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatal("expected close error")
	}
	closeErr, ok := err.(*websocket.CloseError)
	if !ok || closeErr.Code != 4001 {
		t.Fatalf("expected close code 4001, got %v", err)
	}
}

func TestAuthPrivateHandlerConnectionLimit(t *testing.T) {
	hub := NewHubWithMaxConnections(1)
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			if req.APIKey == "limit-key" && req.Signature == "limit-secret" {
				return 55, 0, nil
			}
			return 0, 0, errInvalidAPIKey
		},
	}

	server := httptest.NewServer(PrivateHandler(hub, authCfg))
	defer server.Close()

	conn1 := dialPrivate(t, server.URL, "limit-key", "limit-secret")
	defer conn1.Close()

	conn2 := dialPrivate(t, server.URL, "limit-key", "limit-secret")
	defer conn2.Close()

	conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := conn2.ReadMessage(); err == nil {
		t.Fatal("expected connection to close for max connections")
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("expected close error, got timeout")
	}
}

func TestAuthHubCloseAll(t *testing.T) {
	hub := NewHub()
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			if req.APIKey == "close-key" && req.Signature == "close-secret" {
				return 77, 0, nil
			}
			return 0, 0, errInvalidAPIKey
		},
	}

	server := httptest.NewServer(PrivateHandler(hub, authCfg))
	defer server.Close()

	conn := dialPrivate(t, server.URL, "close-key", "close-secret")
	defer conn.Close()

	waitFor(t, func() bool {
		return hub.ConnectionCount() == 1
	})

	hub.CloseAll()

	waitFor(t, func() bool {
		return hub.ConnectionCount() == 0
	})
}

var errInvalidAPIKey = fmt.Errorf("invalid api key")

func dialPrivate(t *testing.T, serverURL, apiKey, secret string) *websocket.Conn {
	timestamp := time.Now().UnixMilli()
	values := url.Values{}
	values.Set(queryAPIKey, apiKey)
	values.Set(queryTimestamp, strconv.FormatInt(timestamp, 10))
	values.Set(queryNonce, "nonce")
	values.Set(querySignature, secret)

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws/private?" + values.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	return conn
}

func waitFor(t *testing.T, condition func() bool) {
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
