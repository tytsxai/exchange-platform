package response

import (
	"log"
	"net/http"
	"runtime/debug"

	commonerrors "github.com/exchange/common/pkg/errors"
)

type statusWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

// RecoveryMiddleware prevents panics from crashing the process and returns a safe 500 response.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &statusWriter{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic recovered: %v request_id=%s\n%s", v, RequestIDFromRequest(r), string(debug.Stack()))
				if !wrapped.wroteHeader {
					WriteErrorCode(wrapped, r, commonerrors.CodeInternal, "internal server error")
				}
			}
		}()
		next.ServeHTTP(wrapped, r)
	})
}
