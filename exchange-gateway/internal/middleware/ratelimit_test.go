package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute)
	if rl == nil {
		t.Fatal("expected non-nil rate limiter")
	}
	if rl.limit != 100 {
		t.Fatalf("expected limit=100, got %d", rl.limit)
	}
	if rl.window != time.Minute {
		t.Fatalf("expected window=1m, got %v", rl.window)
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.Allow("test-key") {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	// 4th request should be denied
	if rl.Allow("test-key") {
		t.Fatal("expected 4th request to be denied")
	}
}

func TestRateLimiterDifferentKeys(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)

	if !rl.Allow("key1") {
		t.Fatal("expected key1 first request to be allowed")
	}
	if rl.Allow("key1") {
		t.Fatal("expected key1 second request to be denied")
	}

	// Different key should have its own limit
	if !rl.Allow("key2") {
		t.Fatal("expected key2 first request to be allowed")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	rl := NewRateLimiter(1, 10*time.Millisecond)

	if !rl.Allow("test-key") {
		t.Fatal("expected first request to be allowed")
	}
	if rl.Allow("test-key") {
		t.Fatal("expected second request to be denied")
	}

	// Wait for window to reset
	time.Sleep(15 * time.Millisecond)

	if !rl.Allow("test-key") {
		t.Fatal("expected request after window reset to be allowed")
	}
}

func TestBucketStruct(t *testing.T) {
	b := &bucket{
		count:   5,
		resetAt: time.Now().Add(time.Minute),
	}

	if b.count != 5 {
		t.Fatalf("expected count=5, got %d", b.count)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	handler := RateLimit(rl, IPKeyFunc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for request %d, got %d", i+1, rec.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}

	// Check Retry-After header
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected Retry-After=1, got %s", rec.Header().Get("Retry-After"))
	}
}

func TestIPKeyFunc(t *testing.T) {
	// Test with X-Forwarded-For
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "192.168.1.1:12345"

	key := IPKeyFunc(req)
	if key != "10.0.0.1" {
		t.Fatalf("expected key=10.0.0.1, got %s", key)
	}

	// Test without X-Forwarded-For
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	key = IPKeyFunc(req)
	if key != "192.168.1.1:12345" {
		t.Fatalf("expected key=192.168.1.1:12345, got %s", key)
	}
}

func TestUserKeyFunc(t *testing.T) {
	// Test with user ID in context
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// Without user ID, should fall back to IP
	key := UserKeyFunc(req)
	if key != "192.168.1.1:12345" {
		t.Fatalf("expected fallback to IP, got %s", key)
	}
}

func TestRateLimitMiddlewareWithUserKey(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute)

	handler := RateLimit(rl, UserKeyFunc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
