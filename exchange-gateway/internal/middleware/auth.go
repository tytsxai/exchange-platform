// Package middleware 中间件
package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	commonerrors "github.com/exchange/common/pkg/errors"
	commonresp "github.com/exchange/common/pkg/response"
)

type AuthConfig struct {
	TimeWindow      time.Duration
	GetSecret       func(apiKey string) (secret string, userID int64, permissions int, err error)
	VerifySignature func(ctx context.Context, req *VerifySignatureRequest) (userID int64, permissions int, err error)
	WhitelistPaths  map[string]struct{}
	AllowLegacyBody bool
}

type VerifySignatureRequest struct {
	APIKey    string
	Timestamp int64
	Nonce     string
	Signature string
	Method    string
	Path      string
	Query     map[string][]string
	Body      []byte
	BodyHash  string
	ClientIP  string
}

var defaultWhitelist = map[string]struct{}{
	"/health":       {},
	"/ready":        {},
	"/docs":         {},
	"/openapi.yaml": {},
	"/v1/ping":      {},
}

// Auth 鉴权中间件
func Auth(cfg *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isWhitelistedPath(r.URL.Path, cfg) {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := r.Header.Get("X-API-KEY")
			timestampStr := r.Header.Get("X-API-TIMESTAMP")
			nonce := r.Header.Get("X-API-NONCE")
			signature := r.Header.Get("X-API-SIGNATURE")

			if apiKey == "" || timestampStr == "" || nonce == "" || signature == "" {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "missing auth headers")
				return
			}

			timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
			if err != nil {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidTimestamp, "invalid timestamp")
				return
			}

			var userID int64
			var permissions int

			switch {
			case cfg != nil && cfg.VerifySignature != nil:
				body, bodyHash, err := readBodyForSignature(r)
				if err != nil {
					if isRequestTooLarge(err) {
						commonresp.WriteErrorCode(w, r, commonerrors.CodeRequestTooLarge, "")
					} else {
						commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid body")
					}
					return
				}
				userID, permissions, err = cfg.VerifySignature(r.Context(), &VerifySignatureRequest{
					APIKey:    apiKey,
					Timestamp: timestamp,
					Nonce:     nonce,
					Signature: signature,
					Method:    r.Method,
					Path:      r.URL.Path,
					Query:     r.URL.Query(),
					Body:      body,
					BodyHash:  bodyHash,
					ClientIP:  ClientIPFromRequest(r),
				})
				if err != nil && cfg.AllowLegacyBody && len(body) == 0 && bodyHash == "" {
					userID, permissions, err = cfg.VerifySignature(r.Context(), &VerifySignatureRequest{
						APIKey:    apiKey,
						Timestamp: timestamp,
						Nonce:     nonce,
						Signature: signature,
						Method:    r.Method,
						Path:      r.URL.Path,
						Query:     r.URL.Query(),
						Body:      nil,
						BodyHash:  "",
						ClientIP:  ClientIPFromRequest(r),
					})
				}
				if err != nil {
					code, msg := mapAuthError(err)
					commonresp.WriteErrorCode(w, r, code, msg)
					return
				}
			case cfg != nil && cfg.GetSecret != nil:
				window := cfg.TimeWindow
				if window <= 0 {
					window = 30 * time.Second
				}
				now := time.Now().UnixMilli()
				diff := now - timestamp
				if diff < 0 {
					diff = -diff
				}
				if diff > window.Milliseconds() {
					commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidTimestamp, "timestamp expired")
					return
				}

				secret, id, perms, err := cfg.GetSecret(apiKey)
				if err != nil {
					commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidApiKey, "invalid api key")
					return
				}
				canonical := buildCanonicalString(timestamp, nonce, r.Method, r.URL.Path, r.URL.Query())
				expectedSig := sign(secret, canonical)
				if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
					commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidSignature, "invalid signature")
					return
				}
				userID, permissions = id, perms
			default:
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "auth not configured")
				return
			}

			// 设置上下文
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = context.WithValue(ctx, apiKeyKey, apiKey)
			ctx = context.WithValue(ctx, permissionsKey, permissions)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type contextKey string

const (
	userIDKey      contextKey = "userID"
	apiKeyKey      contextKey = "apiKey"
	permissionsKey contextKey = "permissions"
)

// GetUserID 从上下文获取用户 ID
func GetUserID(ctx context.Context) int64 {
	if v := ctx.Value(userIDKey); v != nil {
		return v.(int64)
	}
	return 0
}

// GetAPIKey 从上下文获取 API Key
func GetAPIKey(ctx context.Context) string {
	if v := ctx.Value(apiKeyKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetPermissions 从上下文获取权限
func GetPermissions(ctx context.Context) int {
	if v := ctx.Value(permissionsKey); v != nil {
		return v.(int)
	}
	return 0
}

// HasPermission 检查权限
func HasPermission(ctx context.Context, perm int) bool {
	return GetPermissions(ctx)&perm != 0
}

// 权限常量
const (
	PermRead     = 1
	PermTrade    = 2
	PermWithdraw = 4
)

func buildCanonicalString(timestamp int64, nonce, method, path string, query map[string][]string) string {
	parts := []string{
		fmt.Sprintf("%d", timestamp),
		nonce,
		strings.ToUpper(method),
		path,
		canonicalQuery(query),
	}
	return strings.Join(parts, "\n")
}

func canonicalQuery(query map[string][]string) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		values := query[k]
		sort.Strings(values)
		for _, v := range values {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return strings.Join(pairs, "&")
}

func sign(secret, data string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func readBodyForSignature(r *http.Request) ([]byte, string, error) {
	if r.Body == nil {
		return nil, "", nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, computeBodyHash(body), nil
}

func computeBodyHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func isRequestTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func mapAuthError(err error) (commonerrors.Code, string) {
	if err == nil {
		return commonerrors.CodeInvalidSignature, commonerrors.DefaultMessage(commonerrors.CodeInvalidSignature)
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "invalid timestamp") || strings.Contains(msg, "timestamp expired"):
		return commonerrors.CodeInvalidTimestamp, "invalid timestamp"
	case strings.Contains(msg, "nonce") && strings.Contains(msg, "reused"):
		return commonerrors.CodeInvalidNonce, "invalid nonce"
	case strings.Contains(msg, "user disabled"):
		return commonerrors.CodeUserDisabled, "user disabled"
	case strings.Contains(msg, "user frozen"):
		return commonerrors.CodeUserFrozen, "user frozen"
	case strings.Contains(msg, "invalid api key"):
		return commonerrors.CodeInvalidApiKey, "invalid api key"
	case strings.Contains(msg, "ip not allowed") || strings.Contains(msg, "ip not whitelisted"):
		return commonerrors.CodeIpNotWhitelisted, "ip not whitelisted"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "timeout"):
		return commonerrors.CodeTimeout, "auth timeout"
	case strings.Contains(msg, "user service") || strings.Contains(msg, "service"):
		return commonerrors.CodeUnavailable, "auth service unavailable"
	default:
		return commonerrors.CodeInvalidSignature, "invalid signature"
	}
}

func ClientIPFromRequest(r *http.Request) string {
	clientIP := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		clientIP = host
	}
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff == "" || !IsTrustedProxyIP(clientIP) {
		return clientIP
	}
	if idx := strings.Index(xff, ","); idx >= 0 {
		xff = xff[:idx]
	}
	xff = strings.TrimSpace(xff)
	if xff == "" {
		return clientIP
	}
	return xff
}

func isWhitelistedPath(path string, cfg *AuthConfig) bool {
	if cfg != nil && cfg.WhitelistPaths != nil {
		_, ok := cfg.WhitelistPaths[path]
		return ok
	}
	_, ok := defaultWhitelist[path]
	return ok
}
