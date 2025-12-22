package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	loginIPLimit     = 10
	loginUserLimit   = 5
	loginLimitWindow = time.Minute
	loginFailLimit   = 5
	loginLockTTL     = 15 * time.Minute
)

type LoginRateLimiter struct {
	rdb          redis.Cmdable
	ipLimit      int
	userLimit    int
	window       time.Duration
	failLimit    int
	lockDuration time.Duration
}

func NewLoginRateLimiter(rdb redis.Cmdable) *LoginRateLimiter {
	return &LoginRateLimiter{
		rdb:          rdb,
		ipLimit:      loginIPLimit,
		userLimit:    loginUserLimit,
		window:       loginLimitWindow,
		failLimit:    loginFailLimit,
		lockDuration: loginLockTTL,
	}
}

func (l *LoginRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		email := extractEmail(bodyBytes)
		ip := clientIP(r)

		if email != "" {
			locked, err := l.isLocked(ctx, email)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if locked {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
		}

		ipCount, err := l.incrementCounter(ctx, "login:rate:ip:"+ip, l.window)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if ipCount > int64(l.ipLimit) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		if email != "" {
			userCount, err := l.incrementCounter(ctx, "login:rate:user:"+email, l.window)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if userCount > int64(l.userLimit) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		if email == "" {
			return
		}

		switch rec.status {
		case http.StatusUnauthorized:
			if err := l.recordFailure(ctx, email); err != nil {
				return
			}
		case http.StatusOK:
			_ = l.rdb.Del(ctx, "login:fail:user:"+email).Err()
		}
	})
}

func (l *LoginRateLimiter) incrementCounter(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		if err := l.rdb.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func (l *LoginRateLimiter) isLocked(ctx context.Context, email string) (bool, error) {
	result, err := l.rdb.Exists(ctx, "login:lock:user:"+email).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

func (l *LoginRateLimiter) recordFailure(ctx context.Context, email string) error {
	key := "login:fail:user:" + email
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		if err := l.rdb.Expire(ctx, key, l.lockDuration).Err(); err != nil {
			return err
		}
	}
	if count >= int64(l.failLimit) {
		if err := l.rdb.Set(ctx, "login:lock:user:"+email, "1", l.lockDuration).Err(); err != nil {
			return err
		}
	}
	return nil
}

func extractEmail(body []byte) string {
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	email := strings.TrimSpace(payload.Email)
	if email == "" {
		return ""
	}
	return strings.ToLower(email)
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
