package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequirePermission(t *testing.T) {
	nextCalled := false
	handler := RequirePermission(PermTrade)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/order", nil)
	req = req.WithContext(context.WithValue(req.Context(), permissionsKey, PermRead))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("next should not be called when permission is missing")
	}

	nextCalled = false
	req = httptest.NewRequest(http.MethodPost, "/v1/order", nil)
	req = req.WithContext(context.WithValue(req.Context(), permissionsKey, PermTrade|PermRead))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !nextCalled {
		t.Fatal("next should be called when permission is present")
	}
}

func TestRequirePermissionByMethod(t *testing.T) {
	handler := RequirePermissionByMethod(map[string]int{
		http.MethodGet:    PermRead,
		http.MethodPost:   PermTrade,
		http.MethodDelete: PermTrade,
	}, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET + READ => allow
	req := httptest.NewRequest(http.MethodGet, "/v1/order", nil)
	req = req.WithContext(context.WithValue(req.Context(), permissionsKey, PermRead))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// POST + READ => deny
	req = httptest.NewRequest(http.MethodPost, "/v1/order", nil)
	req = req.WithContext(context.WithValue(req.Context(), permissionsKey, PermRead))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}

	// PATCH not configured + defaultPerm=0 => pass through
	req = httptest.NewRequest(http.MethodPatch, "/v1/order", nil)
	req = req.WithContext(context.WithValue(req.Context(), permissionsKey, 0))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
