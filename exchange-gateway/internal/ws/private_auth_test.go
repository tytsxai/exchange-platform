package ws

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/exchange/gateway/internal/middleware"
)

func TestAuthAuthenticateRequestQueryAuth(t *testing.T) {
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			if req.APIKey == "query-key" && req.Signature == "good-signature" {
				return 9, 0, nil
			}
			return 0, 0, errInvalidSignature
		},
	}

	timestamp := time.Now().UnixMilli()
	values := url.Values{}
	values.Set("symbol", "BTC-USDT")

	req := httptest.NewRequest("GET", "/ws/private?"+values.Encode(), nil)
	query := req.URL.Query()
	query.Set(queryAPIKey, "query-key")
	query.Set(queryTimestamp, strconv.FormatInt(timestamp, 10))
	query.Set(queryNonce, "nonce")
	query.Set(querySignature, "good-signature")
	req.URL.RawQuery = query.Encode()

	userID, err := authenticateRequest(req, authCfg)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if userID != 9 {
		t.Fatalf("userID = %d, want 9", userID)
	}
}

func TestAuthAuthenticateRequestInvalidTimestamp(t *testing.T) {
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			return 1, 0, nil
		},
	}

	req := httptest.NewRequest("GET", "/ws/private", nil)
	query := req.URL.Query()
	query.Set(queryAPIKey, "test")
	query.Set(queryTimestamp, "not-a-number")
	query.Set(queryNonce, "nonce")
	query.Set(querySignature, "sig")
	req.URL.RawQuery = query.Encode()

	_, err := authenticateRequest(req, authCfg)
	if err == nil {
		t.Fatal("expected invalid timestamp error")
	}
}

func TestAuthAuthenticateRequestMissingAuth(t *testing.T) {
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			return 1, 0, nil
		},
	}

	req := httptest.NewRequest("GET", "/ws/private", nil)
	_, err := authenticateRequest(req, authCfg)
	if err == nil {
		t.Fatal("expected missing auth error")
	}
}

func TestAuthAuthenticateRequestExpiredTimestamp(t *testing.T) {
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			return 1, 0, nil
		},
	}

	req := httptest.NewRequest("GET", "/ws/private", nil)
	query := req.URL.Query()
	query.Set(queryAPIKey, "test")
	query.Set(queryTimestamp, strconv.FormatInt(time.Now().Add(-2*time.Minute).UnixMilli(), 10))
	query.Set(queryNonce, "nonce")
	query.Set(querySignature, "sig")
	req.URL.RawQuery = query.Encode()

	_, err := authenticateRequest(req, authCfg)
	if err == nil {
		t.Fatal("expected timestamp expired error")
	}
}

func TestAuthAuthenticateRequestInvalidSignature(t *testing.T) {
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			return 0, 0, errInvalidSignature
		},
	}

	req := httptest.NewRequest("GET", "/ws/private", nil)
	query := req.URL.Query()
	query.Set(queryAPIKey, "test")
	query.Set(queryTimestamp, strconv.FormatInt(time.Now().UnixMilli(), 10))
	query.Set(queryNonce, "nonce")
	query.Set(querySignature, "bad")
	req.URL.RawQuery = query.Encode()

	_, err := authenticateRequest(req, authCfg)
	if err == nil {
		t.Fatal("expected invalid signature error")
	}
}

var errInvalidSignature = fmt.Errorf("invalid signature")

func TestAuthAuthenticateRequestTimeout(t *testing.T) {
	origTimeout := authTimeout
	authTimeout = 10 * time.Millisecond
	defer func() {
		authTimeout = origTimeout
	}()

	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			<-ctx.Done()
			return 0, 0, ctx.Err()
		},
	}

	req := httptest.NewRequest("GET", "/ws/private", nil)
	query := req.URL.Query()
	query.Set(queryAPIKey, "test")
	query.Set(queryTimestamp, strconv.FormatInt(time.Now().UnixMilli(), 10))
	query.Set(queryNonce, "nonce")
	query.Set(querySignature, "sig")
	req.URL.RawQuery = query.Encode()

	_, err := authenticateRequest(req, authCfg)
	if err == nil {
		t.Fatal("expected auth timeout error")
	}
}

func TestAuthSignatureHelpers(t *testing.T) {
	query := map[string][]string{
		"b":            {"2", "1"},
		"a":            {"z"},
		querySignature: {"should-drop"},
	}
	cloned := cloneQueryWithoutSignature(query)
	if _, ok := cloned[querySignature]; ok {
		t.Fatal("expected signature removed")
	}

	canonical := canonicalQuery(cloned)
	if canonical != "a=z&b=1&b=2" {
		t.Fatalf("canonicalQuery = %s", canonical)
	}

	value := buildCanonicalString(123, "n1", "get", "/ws/private", cloned)
	if !strings.Contains(value, "\n") {
		t.Fatal("expected canonical string with separators")
	}

	sig := sign("secret", value)
	if len(sig) != 64 {
		t.Fatalf("signature length = %d", len(sig))
	}
	if sig == sign("secret", value+"x") {
		t.Fatal("expected signature to differ with input change")
	}
}
