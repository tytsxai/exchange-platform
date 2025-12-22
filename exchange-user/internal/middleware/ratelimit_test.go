package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

func requestWithBody(t *testing.T, url string, payload map[string]string) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.RemoteAddr = "10.0.0.1:12345"
	return req
}

func TestLoginRateLimiter_IPLimit(t *testing.T) {
	mr, rdb := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	limiter := &LoginRateLimiter{
		rdb:          rdb,
		ipLimit:      10,
		userLimit:    100,
		window:       time.Minute,
		failLimit:    5,
		lockDuration: 15 * time.Minute,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := limiter.Middleware(handler)

	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := requestWithBody(t, "/v1/auth/login", map[string]string{})
		wrapped.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := requestWithBody(t, "/v1/auth/login", map[string]string{})
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}
}

func TestLoginRateLimiter_UserLimit(t *testing.T) {
	mr, rdb := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	limiter := &LoginRateLimiter{
		rdb:          rdb,
		ipLimit:      100,
		userLimit:    5,
		window:       time.Minute,
		failLimit:    5,
		lockDuration: 15 * time.Minute,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	wrapped := limiter.Middleware(handler)

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "USER@example.com"})
		wrapped.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "user@example.com"})
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}
}

func TestLoginRateLimiter_FailureLockout(t *testing.T) {
	mr, rdb := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	limiter := &LoginRateLimiter{
		rdb:          rdb,
		ipLimit:      100,
		userLimit:    100,
		window:       time.Minute,
		failLimit:    5,
		lockDuration: 15 * time.Minute,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	wrapped := limiter.Middleware(handler)

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "lock@example.com"})
		wrapped.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "lock@example.com"})
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}
}

func TestLoginRateLimiter_SuccessResetsFailures(t *testing.T) {
	mr, rdb := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	limiter := &LoginRateLimiter{
		rdb:          rdb,
		ipLimit:      100,
		userLimit:    100,
		window:       time.Minute,
		failLimit:    5,
		lockDuration: 15 * time.Minute,
	}

	counter := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		switch {
		case counter <= 4:
			w.WriteHeader(http.StatusUnauthorized)
		case counter == 5:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
	wrapped := limiter.Middleware(handler)

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "reset@example.com"})
		wrapped.ServeHTTP(rec, req)
		expected := http.StatusUnauthorized
		if i == 4 {
			expected = http.StatusOK
		}
		if rec.Code != expected {
			t.Fatalf("expected status %d, got %d", expected, rec.Code)
		}
	}

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "reset@example.com"})
		wrapped.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "reset@example.com"})
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "192.168.1.5:4567"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.2")
	if ip := clientIP(req); ip != "203.0.113.9" {
		t.Fatalf("expected forwarded IP, got %s", ip)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "192.168.1.5:4567"
	req.Header.Set("X-Real-IP", "198.51.100.8")
	if ip := clientIP(req); ip != "198.51.100.8" {
		t.Fatalf("expected real IP, got %s", ip)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.77"
	if ip := clientIP(req); ip != "203.0.113.77" {
		t.Fatalf("expected remote IP, got %s", ip)
	}
}

func TestExtractEmail(t *testing.T) {
	if email := extractEmail([]byte(`{"email":"User@Example.com"}`)); email != "user@example.com" {
		t.Fatalf("expected normalized email, got %s", email)
	}
	if email := extractEmail([]byte(`{"email":""}`)); email != "" {
		t.Fatalf("expected empty email, got %s", email)
	}
	if email := extractEmail([]byte(`invalid`)); email != "" {
		t.Fatalf("expected empty email on invalid json, got %s", email)
	}
}

func TestLoginRateLimiter_InvalidBody(t *testing.T) {
	mr, rdb := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	limiter := NewLoginRateLimiter(rdb)
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := limiter.Middleware(handler)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", errReadCloser{})
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if called {
		t.Fatalf("handler should not be called")
	}
}

func TestLoginRateLimiter_RedisErrors(t *testing.T) {
	mr, rdb := newTestRedis(t)
	defer mr.Close()
	rdb.Close()

	limiter := NewLoginRateLimiter(rdb)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := limiter.Middleware(handler)

	req := requestWithBody(t, "/v1/auth/login", map[string]string{"email": "err@example.com"})
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	req = requestWithBody(t, "/v1/auth/login", map[string]string{})
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (errReadCloser) Close() error {
	return nil
}
