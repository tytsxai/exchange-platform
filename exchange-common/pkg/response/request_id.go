package response

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type requestIDKey struct{}

// ContextWithRequestID stores request ID in context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil || requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFromContext reads request ID from context if present.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestIDMiddleware ensures a request ID exists and stores it in context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if reqID == "" {
			reqID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
		}
		if reqID == "" {
			buf := make([]byte, 16)
			if _, err := rand.Read(buf); err == nil {
				reqID = hex.EncodeToString(buf)
			}
		}
		if reqID != "" {
			r.Header.Set("X-Request-ID", reqID)
			w.Header().Set("X-Request-ID", reqID)
			r = r.WithContext(ContextWithRequestID(r.Context(), reqID))
		}
		next.ServeHTTP(w, r)
	})
}
