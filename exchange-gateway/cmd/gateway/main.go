package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	commonerrors "github.com/exchange/common/pkg/errors"
	"github.com/exchange/common/pkg/health"
	"github.com/exchange/common/pkg/logger"
	commonredis "github.com/exchange/common/pkg/redis"
	commonresp "github.com/exchange/common/pkg/response"
	"github.com/exchange/common/pkg/tracing"
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

type redisHealthClient struct {
	client *redis.Client
}

func (c redisHealthClient) Ping(ctx context.Context) health.RedisPingCmd {
	return c.client.Ping(ctx)
}

func main() {
	shutdown, err := tracing.Init(tracing.Config{
		ServiceName: "exchange-gateway",
		Endpoint:    os.Getenv("JAEGER_ENDPOINT"),
		Enabled:     os.Getenv("TRACING_ENABLED") == "true",
		SampleRate:  0.1,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init tracing: %v\n", err)
		os.Exit(1)
	}
	defer shutdown(context.Background())

	cfg := config.Load()
	l := logger.New(cfg.ServiceName, os.Stdout)
	l.Info(fmt.Sprintf("Starting %s...", cfg.ServiceName))

	if err := cfg.Validate(); err != nil {
		l.Error(fmt.Sprintf("Invalid config: %v", err))
		os.Exit(1)
	}
	if err := middleware.SetTrustedProxyCIDRs(cfg.TrustedProxyCIDRs); err != nil {
		l.Error(fmt.Sprintf("Invalid TRUSTED_PROXY_CIDRS: %v", err))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建限流器
	ipLimiter := middleware.NewRateLimiter(cfg.IPRateLimit, time.Second)
	userLimiter := middleware.NewRateLimiter(cfg.UserRateLimit, time.Second)

	// Redis client (private events)
	redisTLSConfig, err := commonredis.TLSConfigFromEnv()
	if err != nil {
		l.Error(fmt.Sprintf("Invalid Redis TLS config: %v", err))
		os.Exit(1)
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		TLSConfig:    redisTLSConfig,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     200,
		MinIdleConns: 20,
	})
	defer redisClient.Close()

	redisPingCtx, redisPingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer redisPingCancel()
	if err := redisClient.Ping(redisPingCtx).Err(); err != nil {
		l.Error(fmt.Sprintf("Failed to connect to Redis: %v", err))
		os.Exit(1)
	}
	l.Info("Connected to Redis")

	// Private events (pub/sub) consumer (powers private websocket push).
	hub := ws.NewHub()
	consumer := ws.NewConsumer(redisClient, hub, cfg.PrivateUserEventChannel)
	var privateEventLoop health.LoopMonitor
	go runPrivateConsumer(ctx, consumer, &privateEventLoop, l)

	// 创建路由
	mux := http.NewServeMux()

	healthz := health.New()
	healthz.Register(health.NewRedisChecker(redisHealthClient{client: redisClient}))
	healthz.Register(newLoopChecker("private_consumer", &privateEventLoop, 45*time.Second))
	healthz.SetReady(true)

	// 公共接口（无需鉴权）
	mux.HandleFunc("/live", healthz.LiveHandler())
	mux.HandleFunc("/health", healthz.HealthHandler())
	metricsHandler := promhttp.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			promhttp.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)
	mux.HandleFunc("/ready", healthz.ReadyHandler())

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
	mux.HandleFunc("/v1/exchangeInfo", proxyHandler(cfg.OrderServiceURL, cfg.InternalToken, l))
	mux.HandleFunc("/v1/depth", proxyHandler(cfg.MarketDataServiceURL, cfg.InternalToken, l))
	mux.HandleFunc("/v1/trades", proxyHandler(cfg.MarketDataServiceURL, cfg.InternalToken, l))
	mux.HandleFunc("/v1/ticker", proxyHandler(cfg.MarketDataServiceURL, cfg.InternalToken, l))

	// 代理到 user 服务 (Auth)
	mux.HandleFunc("/v1/auth/register", proxyHandler(cfg.UserServiceURL, cfg.InternalToken, l))
	mux.HandleFunc("/v1/auth/login", proxyHandler(cfg.UserServiceURL, cfg.InternalToken, l))
	mux.HandleFunc("/v1/apiKeys", proxyHandler(cfg.UserServiceURL, cfg.InternalToken, l))
	mux.HandleFunc("/v1/apiKeys/", proxyHandler(cfg.UserServiceURL, cfg.InternalToken, l))

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
		TimeWindow:      30 * time.Second,
		GetSecret:       nil,
		AllowLegacyBody: true,

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
				Body      string              `json:"body,omitempty"`
				BodyHash  string              `json:"bodyHash,omitempty"`
				ClientIP  string              `json:"clientIp,omitempty"`
			}{
				APIKey:    req.APIKey,
				Timestamp: req.Timestamp,
				Nonce:     req.Nonce,
				Signature: req.Signature,
				Method:    req.Method,
				Path:      req.Path,
				Query:     req.Query,
				Body:      string(req.Body),
				BodyHash:  req.BodyHash,
				ClientIP:  req.ClientIP,
			}

			body, err := json.Marshal(payload)
			if err != nil {
				return 0, 0, fmt.Errorf("marshal payload: %w", err)
			}

			verifyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			verifyURL := strings.TrimRight(cfg.UserServiceURL, "/") + "/internal/verify-signature"
			httpReq, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, verifyURL, bytes.NewReader(body))
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
	wsHandler := ws.PrivateHandler(hub, authCfg, cfg.CORSAllowOrigins)

	// Serve private WS on both HTTP port (backward-compatible) and WS port.
	mux.HandleFunc("/ws/private", wsHandler)
	wsMux := http.NewServeMux()
	wsMux.HandleFunc("/ws/private", wsHandler)

	// 私有接口（需要鉴权）
	privateMux := http.NewServeMux()
	privateMux.Handle("/v1/order",
		middleware.RequirePermissionByMethod(map[string]int{
			http.MethodGet:    middleware.PermRead,
			http.MethodPost:   middleware.PermTrade,
			http.MethodDelete: middleware.PermTrade,
		}, 0)(http.HandlerFunc(proxyHandler(cfg.OrderServiceURL, cfg.InternalToken, l))),
	)
	privateMux.Handle("/v1/openOrders",
		middleware.RequirePermission(middleware.PermRead)(http.HandlerFunc(proxyHandler(cfg.OrderServiceURL, cfg.InternalToken, l))),
	)
	privateMux.Handle("/v1/allOrders",
		middleware.RequirePermission(middleware.PermRead)(http.HandlerFunc(proxyHandler(cfg.OrderServiceURL, cfg.InternalToken, l))),
	)
	privateMux.Handle("/v1/myTrades",
		middleware.RequirePermission(middleware.PermRead)(http.HandlerFunc(proxyHandler(cfg.OrderServiceURL, cfg.InternalToken, l))),
	)
	privateMux.Handle("/v1/account",
		middleware.RequirePermission(middleware.PermRead)(http.HandlerFunc(proxyHandler(cfg.ClearingServiceURL, cfg.InternalToken, l))),
	)
	privateMux.Handle("/v1/ledger",
		middleware.RequirePermission(middleware.PermRead)(http.HandlerFunc(proxyHandler(cfg.ClearingServiceURL, cfg.InternalToken, l))),
	)

	// 组合中间件：
	// 顺序必须是 Auth -> UserRateLimit（依赖已解析出的 userID）。
	rateLimitedPrivate := middleware.RateLimit(userLimiter, middleware.UserKeyFunc)(privateMux)
	authHandler := middleware.Auth(authCfg)(rateLimitedPrivate)

	// 注册私有路由
	mux.Handle("/v1/order", authHandler)
	mux.Handle("/v1/openOrders", authHandler)
	mux.Handle("/v1/allOrders", authHandler)
	mux.Handle("/v1/myTrades", authHandler)
	mux.Handle("/v1/account", authHandler)
	mux.Handle("/v1/ledger", authHandler)

	// 应用 IP 限流到所有请求
	handler := middleware.RateLimit(ipLimiter, middleware.IPKeyFunc)(mux)

	// 添加 CORS 和日志
	handler = corsMiddleware(cfg.CORSAllowOrigins, handler)
	handler = requestIDMiddleware(handler)
	handler = loggingMiddleware(l, handler)
	handler = limitBodyMiddleware(maxBodyBytes, handler)
	handler = tracing.HTTPMiddleware(handler)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	wsServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WSPort),
		Handler:           wsMux,
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

	go func() {
		l.Info(fmt.Sprintf("WS server listening on :%d", cfg.WSPort))
		if err := wsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			l.Error(fmt.Sprintf("WS server error: %v", err))
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
	wsServer.Shutdown(shutdownCtx)
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

func checkConsumeLoop(loop *health.LoopMonitor, name string) dependencyStatus {
	now := time.Now()
	ok, age, _ := loop.Healthy(now, 45*time.Second)
	status := "ok"
	if !ok {
		status = "down"
	}
	if name == "" {
		name = "consumerLoop"
	}
	return dependencyStatus{
		Name:    name,
		Status:  status,
		Latency: age.Milliseconds(),
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
func proxyHandler(targetURL string, internalToken string, l *logger.Logger) http.HandlerFunc {
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
			if l != nil {
				l.Error(fmt.Sprintf("proxy request build error: %v", err))
			}
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInternal, "internal error")
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

		// Preserve/append X-Forwarded-For chain for downstream logging/auditing.
		// Security: only trust the incoming XFF chain when our immediate peer is a trusted proxy.
		clientIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
			clientIP = host
		}
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff == "" || !middleware.IsTrustedProxyIP(clientIP) {
			proxyReq.Header.Set("X-Forwarded-For", clientIP)
		} else {
			proxyReq.Header.Set("X-Forwarded-For", xff+", "+clientIP)
		}
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
			if l != nil {
				l.Error(fmt.Sprintf("proxy request error: %v", err))
			}
			commonresp.WriteStatusError(w, r, http.StatusBadGateway, commonerrors.CodeUnavailable, "bad gateway")
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-KEY, X-API-TIMESTAMP, X-API-NONCE, X-API-SIGNATURE, X-Request-ID")

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
					commonresp.WriteErrorCode(wrapped, r, commonerrors.CodeInternal, "internal server error")
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

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacker not supported")
	}
	return hj.Hijack()
}

func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	p, ok := rw.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
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

const maxBodyBytes int64 = 4 << 20

func limitBodyMiddleware(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && maxBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

type loopChecker struct {
	name   string
	loop   *health.LoopMonitor
	maxAge time.Duration
}

func newLoopChecker(name string, loop *health.LoopMonitor, maxAge time.Duration) health.Checker {
	if name == "" {
		name = "loop"
	}
	return &loopChecker{name: name, loop: loop, maxAge: maxAge}
}

func (c *loopChecker) Name() string {
	if c == nil || c.name == "" {
		return "loop"
	}
	return c.name
}

func (c *loopChecker) Check(ctx context.Context) health.CheckResult {
	if c == nil || c.loop == nil {
		return health.CheckResult{Status: health.StatusDown, Message: "nil loop monitor"}
	}
	ok, age, lastErr := c.loop.Healthy(time.Now(), c.maxAge)
	status := health.StatusUp
	if !ok {
		status = health.StatusDown
	}
	msg := lastErr
	if msg == "" && !ok {
		msg = "stale"
	}
	return health.CheckResult{Status: status, Latency: age, Message: msg}
}

// trusted proxy evaluation lives in middleware.IsTrustedProxyIP

func runPrivateConsumer(ctx context.Context, consumer *ws.Consumer, loop *health.LoopMonitor, l *logger.Logger) {
	const restartDelay = 2 * time.Second
	for {
		err := consumer.RunWithMonitor(ctx, loop)
		if err == nil {
			err = errors.New("private consumer exited")
		}
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}
		if loop != nil {
			loop.SetError(err)
		}
		if l != nil {
			l.Error(fmt.Sprintf("private consumer stopped: %v; restarting in %s", err, restartDelay))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(restartDelay):
		}
	}
}
