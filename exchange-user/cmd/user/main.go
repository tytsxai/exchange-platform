package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/exchange/user/internal/config"
	"github.com/exchange/user/internal/repository"
	"github.com/exchange/user/internal/service"
	_ "github.com/lib/pq"
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

	// HTTP 服务
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
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
	mux.HandleFunc("/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/internal/apiKey", func(w http.ResponseWriter, r *http.Request) {
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
	})

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
