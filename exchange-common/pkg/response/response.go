// Package response provides common HTTP response helpers.
package response

import (
	"encoding/json"
	"net/http"
	"strings"

	commonerrors "github.com/exchange/common/pkg/errors"
)

// RequestIDFromRequest extracts request ID from headers.
func RequestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	reqID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if reqID == "" {
		reqID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
	}
	return reqID
}

// WriteError writes a structured error response based on common error type.
func WriteError(w http.ResponseWriter, r *http.Request, err *commonerrors.Error) {
	if w == nil || err == nil {
		return
	}
	payload := *err
	if reqID := RequestIDFromRequest(r); reqID != "" {
		payload.RequestID = reqID
	}
	writeJSON(w, payload.HTTPStatus(), &payload)
}

// WriteErrorCode writes an error response using error code and message.
func WriteErrorCode(w http.ResponseWriter, r *http.Request, code commonerrors.Code, message string) {
	err := commonerrors.NewWithDefault(code, message)
	WriteError(w, r, err)
}

// WriteStatusError writes an error response with an explicit HTTP status.
func WriteStatusError(w http.ResponseWriter, r *http.Request, status int, code commonerrors.Code, message string) {
	err := commonerrors.NewWithDefault(code, message)
	if w == nil {
		return
	}
	payload := *err
	if reqID := RequestIDFromRequest(r); reqID != "" {
		payload.RequestID = reqID
	}
	writeJSON(w, status, &payload)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
