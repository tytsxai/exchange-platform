// Package middleware 中间件
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AuthConfig 鉴权配置
type AuthConfig struct {
	TimeWindow       time.Duration // 时间窗口（用于本地验签/WS 鉴权）
	GetSecret        func(apiKey string) (secret string, userID int64, permissions int, err error)
	VerifySignature  func(ctx context.Context, req *VerifySignatureRequest) (userID int64, permissions int, err error)
	WhitelistPaths   map[string]struct{}
}

// VerifySignatureRequest 验签请求（供 user 服务 RPC 使用）
type VerifySignatureRequest struct {
	APIKey    string
	Timestamp int64
	Nonce     string
	Signature string
	Method    string
	Path      string
	Query     map[string][]string
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
				http.Error(w, `{"code":"UNAUTHENTICATED","message":"missing auth headers"}`, http.StatusUnauthorized)
				return
			}

			timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
			if err != nil {
				http.Error(w, `{"code":"INVALID_TIMESTAMP","message":"invalid timestamp"}`, http.StatusUnauthorized)
				return
			}

			var userID int64
			var permissions int

			switch {
			case cfg != nil && cfg.VerifySignature != nil:
				userID, permissions, err = cfg.VerifySignature(r.Context(), &VerifySignatureRequest{
					APIKey:    apiKey,
					Timestamp: timestamp,
					Nonce:     nonce,
					Signature: signature,
					Method:    r.Method,
					Path:      r.URL.Path,
					Query:     r.URL.Query(),
				})
				if err != nil {
					http.Error(w, `{"code":"INVALID_SIGNATURE","message":"invalid signature"}`, http.StatusUnauthorized)
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
					http.Error(w, `{"code":"INVALID_TIMESTAMP","message":"timestamp expired"}`, http.StatusUnauthorized)
					return
				}

				secret, id, perms, err := cfg.GetSecret(apiKey)
				if err != nil {
					http.Error(w, `{"code":"INVALID_API_KEY","message":"invalid api key"}`, http.StatusUnauthorized)
					return
				}
				canonical := buildCanonicalString(timestamp, nonce, r.Method, r.URL.Path, r.URL.Query())
				expectedSig := sign(secret, canonical)
				if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
					http.Error(w, `{"code":"INVALID_SIGNATURE","message":"invalid signature"}`, http.StatusUnauthorized)
					return
				}
				userID, permissions = id, perms
			default:
				http.Error(w, `{"code":"UNAUTHENTICATED","message":"auth not configured"}`, http.StatusUnauthorized)
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

func isWhitelistedPath(path string, cfg *AuthConfig) bool {
	if cfg != nil && cfg.WhitelistPaths != nil {
		_, ok := cfg.WhitelistPaths[path]
		return ok
	}
	_, ok := defaultWhitelist[path]
	return ok
}
