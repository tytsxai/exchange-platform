package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/exchange/common/pkg/signature"
	"github.com/exchange/user/internal/config"
	"github.com/exchange/user/internal/middleware"
	"github.com/exchange/user/internal/repository"
	"github.com/exchange/user/internal/service"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// SimpleIDGen 简单 ID 生成器（并发安全）
type SimpleIDGen struct {
	workerID int64
	seq      int64
}

func (g *SimpleIDGen) NextID() int64 {
	seq := atomic.AddInt64(&g.seq, 1)
	return time.Now().UnixNano()/1e6*1000 + g.workerID*100 + seq%100
}

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

	// 连接数据库
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Printf("Connected to PostgreSQL")

	// 创建服务
	idGen := &SimpleIDGen{workerID: cfg.WorkerID}
	repo := repository.NewUserRepository(db)
	svc := service.NewUserService(repo, idGen)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// HTTP 服务
	mux := http.NewServeMux()
	internalToken := os.Getenv("INTERNAL_TOKEN")
	if internalToken == "" {
		internalToken = "internal-secret"
	}
	requireInternalAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Internal-Token") != internalToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), rdb),
		}
		writeHealth(w, deps)
	})
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
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := svc.Register(r.Context(), &service.RegisterRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.ErrorCode != "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"code": resp.ErrorCode})
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
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := svc.Login(r.Context(), &service.LoginRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.ErrorCode != "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"code": resp.ErrorCode})
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
			handleCreateApiKey(w, r, svc)
		case http.MethodGet:
			handleListApiKeys(w, r, svc)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 删除 API Key
	mux.HandleFunc("/v1/apiKeys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleDeleteApiKey(w, r, svc)
	})

	// 内部接口：获取 API Key 信息（供网关调用）
	mux.HandleFunc("/internal/apiKey", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.URL.Query().Get("apiKey")
		if apiKey == "" {
			http.Error(w, "apiKey required", http.StatusBadRequest)
			return
		}

		secretHash, userID, permissions, err := svc.GetApiKeyInfo(r.Context(), apiKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"secretHash":  secretHash,
			"userId":      userID,
			"permissions": permissions,
		})
	}))

	// 内部接口：验证 API Key 签名
	mux.HandleFunc("/internal/verify-signature", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			APIKey    string `json:"apiKey"`
			Timestamp int64  `json:"timestamp"`
			Nonce     string `json:"nonce"`
			Signature string `json:"signature"`
			Method    string `json:"method"`
			Path      string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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

		secret, userID, _, err := svc.GetApiKeyInfo(r.Context(), req.APIKey)
		if err != nil {
			resp["error"] = "invalid api key"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		path := req.Path
		query := url.Values{}
		if parsed, err := url.ParseRequestURI(req.Path); err == nil {
			path = parsed.Path
			query = parsed.Query()
		}

		query.Del("signature")
		canonical := signature.BuildCanonicalString(req.Timestamp, req.Nonce, req.Method, path, query, nil)
		if signature.NewSigner(secret).Verify(canonical, req.Signature) {
			resp["valid"] = true
			resp["userId"] = userID
		} else {
			resp["error"] = "invalid signature"
			resp["userId"] = userID
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: mux,
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

func handleCreateApiKey(w http.ResponseWriter, r *http.Request, svc *service.UserService) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	if userID == 0 {
		http.Error(w, "userId required", http.StatusBadRequest)
		return
	}

	var req struct {
		Label       string   `json:"label"`
		Permissions int      `json:"permissions"`
		IPWhitelist []string `json:"ipWhitelist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.CreateApiKey(r.Context(), &service.CreateApiKeyRequest{
		UserID:      userID,
		Label:       req.Label,
		Permissions: req.Permissions,
		IPWhitelist: req.IPWhitelist,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func handleListApiKeys(w http.ResponseWriter, r *http.Request, svc *service.UserService) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	if userID == 0 {
		http.Error(w, "userId required", http.StatusBadRequest)
		return
	}

	keys, err := svc.ListApiKeys(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func handleDeleteApiKey(w http.ResponseWriter, r *http.Request, svc *service.UserService) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	// 从路径提取 apiKeyId
	path := r.URL.Path
	apiKeyIDStr := path[len("/v1/apiKeys/"):]
	apiKeyID, _ := strconv.ParseInt(apiKeyIDStr, 10, 64)

	if userID == 0 || apiKeyID == 0 {
		http.Error(w, "userId and apiKeyId required", http.StatusBadRequest)
		return
	}

	err := svc.DeleteApiKey(r.Context(), userID, apiKeyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
