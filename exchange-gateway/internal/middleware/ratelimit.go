// Package middleware 限流中间件
package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter 限流器
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string]*bucket
	limit    int
	window   time.Duration
}

type bucket struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter 创建限流器
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*bucket),
		limit:    limit,
		window:   window,
	}
	// 定期清理过期桶
	go rl.cleanup()
	return rl
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.requests[key]

	if !exists || now.After(b.resetAt) {
		rl.requests[key] = &bucket{
			count:   1,
			resetAt: now.Add(rl.window),
		}
		return true
	}

	if b.count >= rl.limit {
		return false
	}

	b.count++
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		rl.cleanupOnce(time.Now())
	}
}

func (rl *RateLimiter) cleanupOnce(now time.Time) {
	rl.mu.Lock()
	for key, b := range rl.requests {
		if now.After(b.resetAt) {
			delete(rl.requests, key)
		}
	}
	rl.mu.Unlock()
}

// RateLimit 限流中间件
func RateLimit(rl *RateLimiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !rl.Allow(key) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, `{"code":"RATE_LIMITED","message":"too many requests","retryable":true}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPKeyFunc 使用 IP 作为限流 key
func IPKeyFunc(r *http.Request) string {
	remoteIP := remoteIPFromAddr(r.RemoteAddr)

	// Security: only trust X-Forwarded-For when the immediate peer is likely a trusted proxy.
	// This prevents direct clients from spoofing XFF and creating unbounded rate-limit keys.
	if remoteIP != "" && isLikelyTrustedProxyIP(remoteIP) {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			// Take the first IP in the chain.
			if idx := strings.IndexByte(xff, ','); idx >= 0 {
				if ip := strings.TrimSpace(xff[:idx]); ip != "" {
					return ip
				}
			}
			return xff
		}
	}

	if remoteIP != "" {
		return remoteIP
	}
	return r.RemoteAddr
}

// UserKeyFunc 使用用户 ID 作为限流 key
func UserKeyFunc(r *http.Request) string {
	userID := GetUserID(r.Context())
	if userID > 0 {
		return strconv.FormatInt(userID, 10)
	}
	return IPKeyFunc(r)
}

func remoteIPFromAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

// isLikelyTrustedProxyIP is a conservative default: loopback or private ranges.
// If your LB/proxy uses public IPs, you should terminate it on a private network
// or add an explicit trust mechanism at the edge.
func isLikelyTrustedProxyIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
