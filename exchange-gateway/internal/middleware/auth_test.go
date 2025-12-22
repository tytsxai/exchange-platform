package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestContextKeyType(t *testing.T) {
	var key contextKey = "test"
	if key != "test" {
		t.Fatalf("expected key=test, got %s", key)
	}
}

func TestContextKeyConstants(t *testing.T) {
	if userIDKey != "userID" {
		t.Fatalf("expected userIDKey=userID, got %s", userIDKey)
	}
	if apiKeyKey != "apiKey" {
		t.Fatalf("expected apiKeyKey=apiKey, got %s", apiKeyKey)
	}
	if permissionsKey != "permissions" {
		t.Fatalf("expected permissionsKey=permissions, got %s", permissionsKey)
	}
}

func TestPermissionConstants(t *testing.T) {
	if PermRead != 1 {
		t.Fatalf("expected PermRead=1, got %d", PermRead)
	}
	if PermTrade != 2 {
		t.Fatalf("expected PermTrade=2, got %d", PermTrade)
	}
	if PermWithdraw != 4 {
		t.Fatalf("expected PermWithdraw=4, got %d", PermWithdraw)
	}
}

func TestGetUserID(t *testing.T) {
	ctx := context.Background()
	if GetUserID(ctx) != 0 {
		t.Fatal("expected 0 for empty context")
	}

	ctx = context.WithValue(ctx, userIDKey, int64(123))
	if GetUserID(ctx) != 123 {
		t.Fatalf("expected userID=123, got %d", GetUserID(ctx))
	}
}

func TestGetAPIKey(t *testing.T) {
	ctx := context.Background()
	if GetAPIKey(ctx) != "" {
		t.Fatal("expected empty string for empty context")
	}

	ctx = context.WithValue(ctx, apiKeyKey, "test-api-key")
	if GetAPIKey(ctx) != "test-api-key" {
		t.Fatalf("expected apiKey=test-api-key, got %s", GetAPIKey(ctx))
	}
}

func TestGetPermissions(t *testing.T) {
	ctx := context.Background()
	if GetPermissions(ctx) != 0 {
		t.Fatal("expected 0 for empty context")
	}

	ctx = context.WithValue(ctx, permissionsKey, 7)
	if GetPermissions(ctx) != 7 {
		t.Fatalf("expected permissions=7, got %d", GetPermissions(ctx))
	}
}

func TestHasPermission(t *testing.T) {
	ctx := context.WithValue(context.Background(), permissionsKey, 7) // 111 in binary

	if !HasPermission(ctx, PermRead) {
		t.Fatal("expected to have PermRead")
	}
	if !HasPermission(ctx, PermTrade) {
		t.Fatal("expected to have PermTrade")
	}
	if !HasPermission(ctx, PermWithdraw) {
		t.Fatal("expected to have PermWithdraw")
	}

	ctx = context.WithValue(context.Background(), permissionsKey, 1) // only read
	if !HasPermission(ctx, PermRead) {
		t.Fatal("expected to have PermRead")
	}
	if HasPermission(ctx, PermTrade) {
		t.Fatal("expected not to have PermTrade")
	}
}

func TestBuildCanonicalString(t *testing.T) {
	timestamp := int64(1000000)
	nonce := "test-nonce"
	method := "GET"
	path := "/api/v1/orders"
	query := map[string][]string{
		"symbol": {"BTCUSDT"},
		"limit":  {"10"},
	}

	canonical := buildCanonicalString(timestamp, nonce, method, path, query)
	if canonical == "" {
		t.Fatal("expected non-empty canonical string")
	}
}

func TestCanonicalQuery(t *testing.T) {
	// Empty query
	result := canonicalQuery(nil)
	if result != "" {
		t.Fatalf("expected empty string for nil query, got %s", result)
	}

	result = canonicalQuery(map[string][]string{})
	if result != "" {
		t.Fatalf("expected empty string for empty query, got %s", result)
	}

	// Single param
	result = canonicalQuery(map[string][]string{"symbol": {"BTCUSDT"}})
	if result != "symbol=BTCUSDT" {
		t.Fatalf("expected symbol=BTCUSDT, got %s", result)
	}

	// Multiple params (should be sorted)
	result = canonicalQuery(map[string][]string{
		"symbol": {"BTCUSDT"},
		"limit":  {"10"},
	})
	if result != "limit=10&symbol=BTCUSDT" {
		t.Fatalf("expected limit=10&symbol=BTCUSDT, got %s", result)
	}
}

func TestSign(t *testing.T) {
	secret := "test-secret"
	data := "test-data"

	sig := sign(secret, data)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	if len(sig) != 64 { // SHA256 hex = 64 chars
		t.Fatalf("expected signature length=64, got %d", len(sig))
	}

	// Same input should produce same output
	sig2 := sign(secret, data)
	if sig != sig2 {
		t.Fatal("expected same signature for same input")
	}

	// Different input should produce different output
	sig3 := sign(secret, "different-data")
	if sig == sig3 {
		t.Fatal("expected different signature for different input")
	}
}

func TestAuthConfigStruct(t *testing.T) {
	cfg := &AuthConfig{
		TimeWindow: 5 * time.Minute,
		GetSecret: func(apiKey string) (string, int64, int, error) {
			return "secret", 123, 7, nil
		},
		VerifySignature: func(ctx context.Context, req *VerifySignatureRequest) (int64, int, error) {
			if req.APIKey != "test" {
				return 0, 0, fmt.Errorf("invalid api key")
			}
			return 123, 7, nil
		},
	}

	if cfg.TimeWindow != 5*time.Minute {
		t.Fatalf("expected TimeWindow=5m, got %v", cfg.TimeWindow)
	}

	secret, userID, perms, err := cfg.GetSecret("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secret != "secret" {
		t.Fatalf("expected secret=secret, got %s", secret)
	}
	if userID != 123 {
		t.Fatalf("expected userID=123, got %d", userID)
	}
	if perms != 7 {
		t.Fatalf("expected perms=7, got %d", perms)
	}

	gotUserID, gotPerms, err := cfg.VerifySignature(context.Background(), &VerifySignatureRequest{
		APIKey:    "test",
		Timestamp: 12345,
		Nonce:     "nonce",
		Signature: "sig",
		Method:    "GET",
		Path:      "/v1/order",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUserID != 123 {
		t.Fatalf("expected userID=123, got %d", gotUserID)
	}
	if gotPerms != 7 {
		t.Fatalf("expected perms=7, got %d", gotPerms)
	}
}

func TestAuthMiddlewareMissingHeaders(t *testing.T) {
	cfg := &AuthConfig{
		VerifySignature: func(ctx context.Context, req *VerifySignatureRequest) (int64, int, error) {
			return 123, 7, nil
		},
	}

	handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareInvalidTimestamp(t *testing.T) {
	cfg := &AuthConfig{
		VerifySignature: func(ctx context.Context, req *VerifySignatureRequest) (int64, int, error) {
			return 123, 7, nil
		},
	}

	handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-KEY", "test-key")
	req.Header.Set("X-API-TIMESTAMP", "invalid")
	req.Header.Set("X-API-NONCE", "nonce")
	req.Header.Set("X-API-SIGNATURE", "test-sig")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareWhitelist(t *testing.T) {
	called := false
	cfg := &AuthConfig{
		VerifySignature: func(ctx context.Context, req *VerifySignatureRequest) (int64, int, error) {
			called = true
			return 123, 7, nil
		},
	}

	handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if called {
		t.Fatal("expected verify not called for whitelisted path")
	}
}

func TestAuthMiddlewareVerifySignatureCalled(t *testing.T) {
	var gotReq *VerifySignatureRequest
	cfg := &AuthConfig{
		VerifySignature: func(ctx context.Context, req *VerifySignatureRequest) (int64, int, error) {
			gotReq = req
			return 99, 3, nil
		},
	}

	handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != 99 {
			t.Fatalf("expected userID=99, got %d", userID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/order", nil)
	req.Header.Set("X-API-KEY", "test-key")
	req.Header.Set("X-API-TIMESTAMP", "1700000000000")
	req.Header.Set("X-API-NONCE", "nonce")
	req.Header.Set("X-API-SIGNATURE", "test-sig")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if gotReq == nil {
		t.Fatal("expected VerifySignature to be called")
	}
	if gotReq.APIKey != "test-key" {
		t.Fatalf("expected apiKey=test-key, got %s", gotReq.APIKey)
	}
	if gotReq.Method != "POST" {
		t.Fatalf("expected method=POST, got %s", gotReq.Method)
	}
	if gotReq.Path != "/v1/order" {
		t.Fatalf("expected path=/v1/order, got %s", gotReq.Path)
	}
	if gotReq.Nonce != "nonce" {
		t.Fatalf("expected nonce=nonce, got %s", gotReq.Nonce)
	}
}

func TestAuthMiddlewareInvalidSignature(t *testing.T) {
	cfg := &AuthConfig{
		VerifySignature: func(ctx context.Context, req *VerifySignatureRequest) (int64, int, error) {
			return 0, 0, fmt.Errorf("invalid signature")
		},
	}

	handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/account", nil)
	req.Header.Set("X-API-KEY", "test-key")
	req.Header.Set("X-API-TIMESTAMP", "1700000000000")
	req.Header.Set("X-API-NONCE", "nonce")
	req.Header.Set("X-API-SIGNATURE", "bad-sig")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}
