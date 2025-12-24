package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/exchange/common/pkg/logger"
	"github.com/exchange/gateway/internal/config"
	"github.com/exchange/gateway/internal/middleware"
	"github.com/exchange/gateway/internal/ws"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// 全局 HTTP 客户端（复用连接）
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

func main() {
	cfg := config.Load()
	l := logger.New(cfg.ServiceName, os.Stdout)
	l.Info(fmt.Sprintf("Starting %s...", cfg.ServiceName))

	if err := cfg.Validate(); err != nil {
		l.Error(fmt.Sprintf("Invalid config: %v", err))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建限流器
	ipLimiter := middleware.NewRateLimiter(cfg.IPRateLimit, time.Second)
	userLimiter := middleware.NewRateLimiter(cfg.UserRateLimit, time.Second)

	// Redis client (private events)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		l.Error(fmt.Sprintf("Failed to connect to Redis: %v", err))
		os.Exit(1)
	}
	l.Info("Connected to Redis")

	// 创建路由
	mux := http.NewServeMux()
	healthHTTPClient := &http.Client{Timeout: 2 * time.Second}

	// 公共接口（无需鉴权）
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "order", cfg.OrderServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "user", cfg.UserServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "clearing", cfg.ClearingServiceURL, healthHTTPClient),
		}
		writeHealth(w, deps)
	})
	metricsHandler := promhttp.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			promhttp.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "order", cfg.OrderServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "user", cfg.UserServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "clearing", cfg.ClearingServiceURL, healthHTTPClient),
		}
		writeHealth(w, deps)
	})

	mux.HandleFunc("/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})

	mux.HandleFunc("/v1/time", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{
			"serverTime": time.Now().UnixMilli(),
		})
	})

	// 代理到 order 服务
	mux.HandleFunc("/v1/exchangeInfo", proxyHandler(cfg.OrderServiceURL, cfg.InternalToken))
	mux.HandleFunc("/v1/depth", proxyHandler(cfg.MarketDataServiceURL, cfg.InternalToken))
	mux.HandleFunc("/v1/trades", proxyHandler(cfg.MarketDataServiceURL, cfg.InternalToken))
	mux.HandleFunc("/v1/ticker", proxyHandler(cfg.MarketDataServiceURL, cfg.InternalToken))

	// 代理到 user 服务 (Auth)
	mux.HandleFunc("/v1/auth/register", proxyHandler(cfg.UserServiceURL, cfg.InternalToken))
	mux.HandleFunc("/v1/auth/login", proxyHandler(cfg.UserServiceURL, cfg.InternalToken))
	mux.HandleFunc("/v1/apiKeys", proxyHandler(cfg.UserServiceURL, cfg.InternalToken))
	mux.HandleFunc("/v1/apiKeys/", proxyHandler(cfg.UserServiceURL, cfg.InternalToken))

	// Swagger UI - API 文档
	// 访问 /docs 查看交互式 API 文档，支持在线测试
	// 访问 /openapi.yaml 获取 OpenAPI 3.0 规范文件
	if cfg.EnableDocs {
		mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "api/openapi.yaml")
		})
		mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
			html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Exchange API Documentation</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <style>
        body { margin: 0; padding: 0; }
        .swagger-ui .topbar { display: none; }
        .swagger-ui .info { margin: 20px 0; }
        .swagger-ui .info .title { font-size: 2em; }
        .swagger-ui .scheme-container { background: #fafafa; padding: 15px 0; }
        .swagger-ui .btn.authorize { background-color: #49cc90; border-color: #49cc90; }
        .swagger-ui .btn.authorize svg { fill: #fff; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = () => {
            window.ui = SwaggerUIBundle({
                url: '/openapi.yaml',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                tryItOutEnabled: true,
                supportedSubmitMethods: ['get', 'post', 'put', 'delete', 'patch'],
                validatorUrl: null,
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                docExpansion: 'list',
                filter: true,
                showExtensions: true,
                showCommonExtensions: true,
                persistAuthorization: true
            });
        };
    </script>
</body>
</html>`
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(html))
		})
	} else {
		mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
		mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	// 需要鉴权的接口
	authCfg := &middleware.AuthConfig{
		TimeWindow: 30 * time.Second,
		GetSecret:  nil, // 使用 VerifySignature
		VerifySignature: func(ctx context.Context, req *middleware.VerifySignatureRequest) (int64, int, error) {
			// 构造请求
			payload := struct {
				APIKey    string              `json:"apiKey"`
				Timestamp int64               `json:"timestamp"`
				Nonce     string              `json:"nonce"`
				Signature string              `json:"signature"`
				Method    string              `json:"method"`
				Path      string              `json:"path"`
				Query     map[string][]string `json:"query,omitempty"`
			}{
				APIKey:    req.APIKey,
				Timestamp: req.Timestamp,
				Nonce:     req.Nonce,
				Signature: req.Signature,
				Method:    req.Method,
				Path:      req.Path,
				Query:     req.Query,
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return 0, 0, fmt.Errorf("marshal payload: %w", err)
			}

			verifyURL := strings.TrimRight(cfg.UserServiceURL, "/") + "/internal/verify-signature"
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader(body))
			if err != nil {
				return 0, 0, fmt.Errorf("create validation request: %w", err)
			}

			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("X-Internal-Token", cfg.InternalToken)

			resp, err := httpClient.Do(httpReq)
			if err != nil {
				return 0, 0, fmt.Errorf("call user service: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return 0, 0, fmt.Errorf("user service returned %d", resp.StatusCode)
			}

			var result struct {
				Valid       bool   `json:"valid"`
				UserID      int64  `json:"userId"`
				Permissions int    `json:"permissions"`
				Error       string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return 0, 0, fmt.Errorf("decode response: %w", err)
			}

			if !result.Valid {
				return 0, 0, fmt.Errorf("signature validation failed: %s", result.Error)
			}

			return result.UserID, result.Permissions, nil
		},
	}

	// Private WebSocket
	hub := ws.NewHub()
	consumer := ws.NewConsumer(redisClient, hub, cfg.PrivateUserEventChannel)
	go func() {
		if err := consumer.Run(ctx); err != nil && err != context.Canceled {
			l.Error(fmt.Sprintf("private consumer stopped: %v", err))
		}
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.Info(fmt.Sprintf("private ws connections: %d", hub.ConnectionCount()))
			}
		}
	}()
	mux.HandleFunc("/ws/private", ws.PrivateHandler(hub, authCfg))

	// 私有接口（需要鉴权）
	privateMux := http.NewServeMux()
	privateMux.HandleFunc("/v1/order", proxyHandler(cfg.OrderServiceURL, cfg.InternalToken))
	privateMux.HandleFunc("/v1/openOrders", proxyHandler(cfg.OrderServiceURL, cfg.InternalToken))
	privateMux.HandleFunc("/v1/allOrders", proxyHandler(cfg.OrderServiceURL, cfg.InternalToken))
	privateMux.HandleFunc("/v1/myTrades", proxyHandler(cfg.OrderServiceURL, cfg.InternalToken))
	privateMux.HandleFunc("/v1/account", proxyHandler(cfg.ClearingServiceURL, cfg.InternalToken))
	privateMux.HandleFunc("/v1/ledger", proxyHandler(cfg.ClearingServiceURL, cfg.InternalToken))

	// 组合中间件
	authHandler := middleware.Auth(authCfg)(privateMux)
	rateLimitedAuth := middleware.RateLimit(userLimiter, middleware.UserKeyFunc)(authHandler)

	// 注册私有路由
	mux.Handle("/v1/order", rateLimitedAuth)
	mux.Handle("/v1/openOrders", rateLimitedAuth)
	mux.Handle("/v1/allOrders", rateLimitedAuth)
	mux.Handle("/v1/myTrades", rateLimitedAuth)
	mux.Handle("/v1/account", rateLimitedAuth)
	mux.Handle("/v1/ledger", rateLimitedAuth)

	// 应用 IP 限流到所有请求
	handler := middleware.RateLimit(ipLimiter, middleware.IPKeyFunc)(mux)

	// 添加 CORS 和日志
	handler = corsMiddleware(cfg.CORSAllowOrigins, handler)
	handler = requestIDMiddleware(handler)
	handler = loggingMiddleware(l, handler)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		l.Info(fmt.Sprintf("HTTP server listening on :%d", cfg.HTTPPort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			l.Error(fmt.Sprintf("HTTP server error: %v", err))
			os.Exit(1)
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	l.Info("Shutting down...")
	cancel()
	hub.CloseAll()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
	l.Info("Shutdown complete")
}

type dependencyStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Latency int64  `json:"latency"`
}

type healthResponse struct {
	Status       string             `json:"status"`
	Dependencies []dependencyStatus `json:"dependencies"`
}

func checkRedis(ctx context.Context, client *redis.Client) dependencyStatus {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := client.Ping(timeoutCtx).Err()
	status := "ok"
	if err != nil {
		status = "down"
	}
	return dependencyStatus{
		Name:    "redis",
		Status:  status,
		Latency: time.Since(start).Milliseconds(),
	}
}

func checkHTTP(ctx context.Context, name, baseURL string, client *http.Client) dependencyStatus {
	start := time.Now()
	status := "ok"
	if baseURL == "" {
		status = "down"
	} else {
		healthURL := strings.TrimRight(baseURL, "/") + "/health"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			status = "down"
		} else {
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
				status = "down"
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
	return dependencyStatus{
		Name:    name,
		Status:  status,
		Latency: time.Since(start).Milliseconds(),
	}
}

func writeHealth(w http.ResponseWriter, deps []dependencyStatus) {
	status := "ok"
	for _, dep := range deps {
		if dep.Status != "ok" {
			status = "degraded"
			break
		}
	}
	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(healthResponse{
		Status:       status,
		Dependencies: deps,
	})
}

// proxyHandler 创建代理处理器
func proxyHandler(targetURL string, internalToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 构建目标 URL（禁止信任客户端 userId；由网关注入）
		target := targetURL + r.URL.Path
		q := r.URL.Query()
		q.Del("userId")
		encoded := q.Encode()
		if encoded != "" {
			target += "?" + encoded
		}

		// 创建代理请求
		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 复制请求头
		for key, values := range r.Header {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
		// 覆盖敏感头，防止客户端伪造
		proxyReq.Header.Del("X-Internal-Token")
		proxyReq.Header.Del("X-User-Id")
		proxyReq.Header.Del("X-User-ID")

		proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
		proxyReq.Header.Set("X-Request-ID", r.Header.Get("X-Request-ID"))

		// 内部服务鉴权：统一携带 INTERNAL_TOKEN
		// 注意：对 user-service 的 public /v1/auth/* 无影响（它不会校验该头）。
		if internalToken != "" {
			proxyReq.Header.Set("X-Internal-Token", internalToken)
		}

		// 用户身份绑定：下游只信任网关注入的 userId header
		if userID := middleware.GetUserID(r.Context()); userID > 0 {
			proxyReq.Header.Set("X-User-Id", fmt.Sprintf("%d", userID))
		}

		// 发送请求
		resp, err := httpClient.Do(proxyReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// 复制响应头
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// 写入响应
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// corsMiddleware CORS 中间件
func corsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowOrigin := ""
		for _, o := range allowedOrigins {
			if o == "*" {
				allowOrigin = "*"
				break
			}
			if origin != "" && origin == o {
				allowOrigin = origin
				break
			}
		}
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			if allowOrigin != "*" {
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-KEY, X-API-TIMESTAMP, X-API-NONCE, X-API-SIGNATURE, X-Request-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware 日志中间件
func loggingMiddleware(l *logger.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			if v := recover(); v != nil {
				if !wrapped.wroteHeader {
					http.Error(wrapped, "internal server error", http.StatusInternalServerError)
				}
				l.Error(fmt.Sprintf("panic recovered: %v request_id=%s", v, requestIDFromRequest(r)))
			}
			l.Info(fmt.Sprintf("%s %s %d %v request_id=%s", r.Method, r.URL.Path, wrapped.statusCode, time.Since(start), requestIDFromRequest(r)))
		}()

		next.ServeHTTP(wrapped, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func requestIDFromRequest(r *http.Request) string {
	reqID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if reqID == "" {
		reqID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
	}
	if reqID == "" {
		return "-"
	}
	return reqID
}

func requestIDMiddleware(next http.Handler) http.Handler {
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
		}
		next.ServeHTTP(w, r)
	})
}

func metricsAuthorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Metrics-Token")) == token {
		return true
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") && strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == token {
		return true
	}
	return false
}
