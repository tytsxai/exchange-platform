package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubAdminPermissionReader struct {
	perms map[int64][]string
	err   error
}

func (s *stubAdminPermissionReader) GetUserPermissions(ctx context.Context, userID int64) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.perms[userID], nil
}

func TestMatchAdminRoutePermission(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		matched bool
	}{
		{name: "exact path", method: http.MethodPost, path: "/admin/symbols", matched: true},
		{name: "prefix path", method: http.MethodPatch, path: "/admin/symbols/BTCUSDT", matched: true},
		{name: "unknown path", method: http.MethodGet, path: "/admin/unknown", matched: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, matched := matchAdminRoutePermission(tc.method, tc.path)
			if matched != tc.matched {
				t.Fatalf("expected matched=%v, got %v", tc.matched, matched)
			}
		})
	}
}

func TestHasAnyPermission(t *testing.T) {
	if !hasAnyPermission([]string{"*"}, []string{"symbol:write"}) {
		t.Fatalf("wildcard permission should pass")
	}
	if !hasAnyPermission([]string{"symbol:read", "risk:write"}, []string{"symbol:write", "risk:write"}) {
		t.Fatalf("expected any permission match")
	}
	if hasAnyPermission([]string{"audit:read"}, []string{"symbol:write"}) {
		t.Fatalf("unexpected permission match")
	}
}

func TestAdminPermissionMiddleware_AllowAndDeny(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	reader := &stubAdminPermissionReader{
		perms: map[int64][]string{
			100: {"risk:write"},
			101: {"audit:read"},
		},
	}
	handler := adminPermissionMiddleware(reader, next)

	allowReq := httptest.NewRequest(http.MethodPost, "/admin/killSwitch", nil)
	allowReq.Header.Set("X-Actor-ID", "100")
	allowResp := httptest.NewRecorder()
	handler.ServeHTTP(allowResp, allowReq)
	if allowResp.Code != http.StatusNoContent {
		t.Fatalf("expected allow status %d, got %d", http.StatusNoContent, allowResp.Code)
	}

	denyReq := httptest.NewRequest(http.MethodPost, "/admin/killSwitch", nil)
	denyReq.Header.Set("X-Actor-ID", "101")
	denyResp := httptest.NewRecorder()
	handler.ServeHTTP(denyResp, denyReq)
	if denyResp.Code != http.StatusForbidden {
		t.Fatalf("expected deny status %d, got %d", http.StatusForbidden, denyResp.Code)
	}
}

func TestAdminPermissionMiddleware_FailClosed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	reader := &stubAdminPermissionReader{
		perms: map[int64][]string{
			100: {"*"},
		},
	}
	handler := adminPermissionMiddleware(reader, next)

	req := httptest.NewRequest(http.MethodGet, "/admin/newFeature", nil)
	req.Header.Set("X-Actor-ID", "100")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for unmatched route, got %d", resp.Code)
	}
}

func TestAdminPermissionMiddleware_InternalError(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	reader := &stubAdminPermissionReader{
		err: errors.New("db down"),
	}
	handler := adminPermissionMiddleware(reader, next)

	req := httptest.NewRequest(http.MethodGet, "/admin/auditLogs", nil)
	req.Header.Set("X-Actor-ID", "100")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected internal error status, got %d", resp.Code)
	}
}
