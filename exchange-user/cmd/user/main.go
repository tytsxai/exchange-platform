package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/exchange/common/pkg/audit"
	commonauth "github.com/exchange/common/pkg/auth"
	commonerrors "github.com/exchange/common/pkg/errors"
	commonredis "github.com/exchange/common/pkg/redis"
	commonresp "github.com/exchange/common/pkg/response"
	"github.com/exchange/common/pkg/signature"
	"github.com/exchange/common/pkg/snowflake"
	"github.com/exchange/user/internal/config"
	"github.com/exchange/user/internal/middleware"
	"github.com/exchange/user/internal/repository"
	"github.com/exchange/user/internal/service"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	tokenManager, err := commonauth.NewTokenManager(cfg.AuthTokenSecret, cfg.AuthTokenTTL)
	if err != nil {
		log.Fatalf("Invalid auth token config: %v", err)
	}

	if err := snowflake.Init(cfg.WorkerID); err != nil {
		log.Fatalf("Failed to init snowflake: %v", err)
	}

	// 连接数据库
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)

	dbPingCtx, dbPingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbPingCancel()
	if err := db.PingContext(dbPingCtx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Printf("Connected to PostgreSQL")

	// 创建服务
	idGen := snowflakeIDGen{}
	repo, err := repository.NewUserRepositoryWithAPIKeySecret(db, []byte(cfg.APIKeySecretKey))
	if err != nil {
		log.Fatalf("Invalid API key secret config: %v", err)
	}
	svc := service.NewUserService(repo, idGen, tokenManager)
	auditLogger, err := audit.NewDBLogger(db, audit.WithErrorHandler(func(auditErr error) {
		log.Printf("audit logger error: %v", auditErr)
	}))
	if err != nil {
		log.Fatalf("Failed to init audit logger: %v", err)
	}
	defer auditLogger.Close()
	svc.SetAuditLogger(auditLogger)

	redisTLSConfig, err := commonredis.TLSConfigFromEnv()
	if err != nil {
		log.Fatalf("Invalid Redis TLS config: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		TLSConfig:    redisTLSConfig,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     100,
		MinIdleConns: 10,
	})
	defer rdb.Close()
	redisPingCtx, redisPingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer redisPingCancel()
	if err := rdb.Ping(redisPingCtx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	nonceStore := commonredis.NewNonceStore(&commonredis.Client{Client: rdb}, cfg.NonceKeyPrefix)

	// HTTP 服务
	mux := http.NewServeMux()
	internalToken := cfg.InternalToken
	requireInternalAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Internal-Token") != internalToken {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			next(w, r)
		}
	}

	// 健康检查
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), rdb),
		}
		writeHealth(w, deps)
	})
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
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), rdb),
		}
		writeHealth(w, deps)
	})

	// 注册
	mux.HandleFunc("/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		resp, err := svc.Register(r.Context(), &service.RegisterRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.ErrorCode != "" {
			commonresp.WriteErrorCode(w, r, commonerrors.Code(resp.ErrorCode), "")
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"userId": resp.User.UserID,
			"email":  resp.User.Email,
		})
	})

	// 登录
	loginHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		resp, err := svc.Login(r.Context(), &service.LoginRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.ErrorCode != "" {
			commonresp.WriteErrorCode(w, r, commonerrors.Code(resp.ErrorCode), "")
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"userId": resp.User.UserID,
			"email":  resp.User.Email,
			"token":  resp.Token,
		})
	})
	loginLimiter := middleware.NewLoginRateLimiter(rdb)
	mux.Handle("/v1/auth/login", loginLimiter.Middleware(loginHandler))

	// 创建 API Key
	mux.HandleFunc("/v1/apiKeys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateApiKey(w, r, svc, tokenManager)
		case http.MethodGet:
			handleListApiKeys(w, r, svc, tokenManager)
		default:
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
		}
	})

	// 删除 API Key
	mux.HandleFunc("/v1/apiKeys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}
		handleDeleteApiKey(w, r, svc, tokenManager)
	})

	// 内部接口：获取 API Key 信息（供网关调用）
	mux.HandleFunc("/internal/apiKey", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.URL.Query().Get("apiKey")
		if apiKey == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "apiKey required")
			return
		}

		secretHash, userID, permissions, ipWhitelist, err := svc.GetApiKeyInfo(r.Context(), apiKey)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrUserFrozen):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUserFrozen, "user frozen")
			case errors.Is(err, service.ErrUserDisabled):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUserDisabled, "user disabled")
			default:
				commonresp.WriteErrorCode(w, r, commonerrors.CodeNotFound, "api key not found")
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"secretHash":  secretHash,
			"userId":      userID,
			"permissions": permissions,
			"ipWhitelist": ipWhitelist,
		})
	}))

	// 内部接口：验证 API Key 签名
	mux.HandleFunc("/internal/verify-signature", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req struct {
			APIKey    string              `json:"apiKey"`
			Timestamp int64               `json:"timestamp"`
			Nonce     string              `json:"nonce"`
			Signature string              `json:"signature"`
			Method    string              `json:"method"`
			Path      string              `json:"path"`
			Query     map[string][]string `json:"query"`
			Body      string              `json:"body"`
			BodyHash  string              `json:"bodyHash"`
			ClientIP  string              `json:"clientIp"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		resp := map[string]interface{}{
			"valid":  false,
			"userId": int64(0),
			"error":  "",
		}
		if req.APIKey == "" || req.Timestamp == 0 || req.Nonce == "" || req.Signature == "" || req.Method == "" || req.Path == "" {
			resp["error"] = "missing required fields"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		secret, userID, permissions, ipWhitelist, err := svc.GetApiKeyInfo(r.Context(), req.APIKey)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrUserFrozen):
				resp["error"] = "user frozen"
			case errors.Is(err, service.ErrUserDisabled):
				resp["error"] = "user disabled"
			default:
				resp["error"] = "invalid api key"
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		clientIP := strings.TrimSpace(req.ClientIP)
		if clientIP == "" || net.ParseIP(clientIP) == nil {
			clientIP = clientIPFromRequest(r)
		}
		if len(ipWhitelist) > 0 && !ipAllowed(clientIP, ipWhitelist) {
			resp["error"] = "ip not allowed"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		path := req.Path
		query := url.Values{}
		if len(req.Query) > 0 {
			query = url.Values(req.Query)
		} else if parsed, err := url.ParseRequestURI(req.Path); err == nil {
			path = parsed.Path
			query = parsed.Query()
		}

		query.Del("signature")
		verifier := signature.NewVerifier(secret, signature.WithNonceStore(nonceStore))
		var bodyBytes []byte
		if req.Body != "" {
			bodyBytes = []byte(req.Body)
		}
		err = verifier.VerifyRequest(&signature.Request{
			ApiKey:      req.APIKey,
			TimestampMs: req.Timestamp,
			Nonce:       req.Nonce,
			Signature:   req.Signature,
			Method:      req.Method,
			Path:        path,
			Query:       query,
			Body:        bodyBytes,
			BodyHash:    req.BodyHash,
		})
		if err == nil {
			resp["valid"] = true
			resp["userId"] = userID
			resp["permissions"] = permissions
		} else {
			resp["userId"] = userID
			switch err {
			case signature.ErrInvalidTimestamp:
				resp["error"] = "invalid timestamp"
			case signature.ErrNonceReused:
				resp["error"] = "nonce reused"
			case signature.ErrInvalidSignature:
				resp["error"] = "invalid signature"
			default:
				resp["error"] = err.Error()
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	handler := limitBodyMiddleware(maxBodyBytes, mux)
	handler = commonresp.RequestIDMiddleware(handler)
	handler = commonresp.RecoveryMiddleware(handler)
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
		log.Printf("HTTP server listening on :%d", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("Shutdown complete")
}

type snowflakeIDGen struct{}

func (g snowflakeIDGen) NextID() int64 {
	return snowflake.MustNextID()
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

func checkPostgres(ctx context.Context, db *sql.DB) dependencyStatus {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := db.PingContext(timeoutCtx)
	status := "ok"
	if err != nil {
		status = "down"
	}
	return dependencyStatus{
		Name:    "postgres",
		Status:  status,
		Latency: time.Since(start).Milliseconds(),
	}
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

func handleCreateApiKey(w http.ResponseWriter, r *http.Request, svc *service.UserService, tokenManager *commonauth.TokenManager) {
	userID, err := userIDFromBearer(r, tokenManager)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
		return
	}

	var req struct {
		Label       string   `json:"label"`
		Permissions int      `json:"permissions"`
		IPWhitelist []string `json:"ipWhitelist"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	resp, err := svc.CreateApiKey(r.Context(), &service.CreateApiKeyRequest{
		UserID:      userID,
		Label:       req.Label,
		Permissions: req.Permissions,
		IPWhitelist: req.IPWhitelist,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"apiKeyId":    resp.ApiKey.ApiKeyID,
		"apiKey":      resp.ApiKey.ApiKey,
		"secret":      resp.Secret,
		"label":       resp.ApiKey.Label,
		"permissions": resp.ApiKey.Permissions,
	})
}

func handleListApiKeys(w http.ResponseWriter, r *http.Request, svc *service.UserService, tokenManager *commonauth.TokenManager) {
	userID, err := userIDFromBearer(r, tokenManager)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
		return
	}

	keys, err := svc.ListApiKeys(r.Context(), userID)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, map[string]interface{}{
			"apiKeyId":    k.ApiKeyID,
			"apiKey":      k.ApiKey,
			"label":       k.Label,
			"permissions": k.Permissions,
			"ipWhitelist": k.IPWhitelist,
			"createdAt":   k.CreatedAtMs,
		})
	}
	json.NewEncoder(w).Encode(resp)
}

func handleDeleteApiKey(w http.ResponseWriter, r *http.Request, svc *service.UserService, tokenManager *commonauth.TokenManager) {
	userID, err := userIDFromBearer(r, tokenManager)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
		return
	}
	// 从路径提取 apiKeyId
	path := r.URL.Path
	apiKeyIDStr := path[len("/v1/apiKeys/"):]
	apiKeyID, _ := strconv.ParseInt(apiKeyIDStr, 10, 64)

	if apiKeyID == 0 {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "apiKeyId required")
		return
	}

	err = svc.DeleteApiKey(r.Context(), userID, apiKeyID)
	if err != nil {
		log.Printf("delete api key error: %v", err)
		commonresp.WriteErrorCode(w, r, commonerrors.CodeNotFound, "api key not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func userIDFromBearer(r *http.Request, tokenManager *commonauth.TokenManager) (int64, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return 0, fmt.Errorf("authorization required")
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return 0, fmt.Errorf("invalid authorization format")
	}
	userID, err := tokenManager.Verify(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid token")
	}
	if userID <= 0 {
		return 0, fmt.Errorf("invalid token")
	}
	return userID, nil
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

func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if isRequestTooLarge(err) {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeRequestTooLarge, "")
			return false
		}
		commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid request")
		return false
	}
	return true
}

func isRequestTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func writeInternalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	commonresp.WriteErrorCode(w, nil, commonerrors.CodeInternal, "internal error")
}

func ipAllowed(clientIP string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}
	for _, entry := range whitelist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err != nil {
				continue
			}
			if cidr.Contains(ip) {
				return true
			}
			continue
		}
		if ip.Equal(net.ParseIP(entry)) {
			return true
		}
	}
	return false
}

func clientIPFromRequest(r *http.Request) string {
	remoteIP := remoteIPFromAddr(r.RemoteAddr)
	if remoteIP != "" && isLikelyTrustedProxyIP(remoteIP) {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			if idx := strings.IndexByte(forwarded, ','); idx >= 0 {
				forwarded = forwarded[:idx]
			}
			if ip := strings.TrimSpace(forwarded); ip != "" {
				return ip
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}
	if remoteIP != "" {
		return remoteIP
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func remoteIPFromAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

func isLikelyTrustedProxyIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
