// Package middleware 权限中间件
package middleware

import (
	"net/http"
	"strings"

	commonerrors "github.com/exchange/common/pkg/errors"
	commonresp "github.com/exchange/common/pkg/response"
)

// RequirePermission 校验上下文中的 API Key 权限位。
func RequirePermission(perm int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if perm <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			if !HasPermission(r.Context(), perm) {
				commonresp.WriteErrorCode(w, r, commonerrors.CodePermissionDenied, "permission denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermissionByMethod 按 HTTP Method 进行权限校验。
//
// methodPerms: method -> required permission
// defaultPerm:
//   - >0: method 未命中时按 defaultPerm 校验
//   - <=0: method 未命中时跳过权限校验（交由下游继续处理）
func RequirePermissionByMethod(methodPerms map[string]int, defaultPerm int) func(http.Handler) http.Handler {
	normalized := make(map[string]int, len(methodPerms))
	for method, perm := range methodPerms {
		normalized[strings.ToUpper(strings.TrimSpace(method))] = perm
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := strings.ToUpper(strings.TrimSpace(r.Method))
			perm, ok := normalized[method]
			if !ok {
				perm = defaultPerm
			}
			if perm > 0 && !HasPermission(r.Context(), perm) {
				commonresp.WriteErrorCode(w, r, commonerrors.CodePermissionDenied, "permission denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
