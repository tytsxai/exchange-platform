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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/config"
	"github.com/exchange/wallet/internal/repository"
	"github.com/exchange/wallet/internal/service"
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
	repo := repository.NewWalletRepository(db)
	clearingBaseURL := "http://localhost:8083" // TODO: use config
	clearingCli := client.NewClearingClient(clearingBaseURL)
	svc := service.NewWalletService(repo, idGen, clearingCli)

	// HTTP 服务
	mux := http.NewServeMux()
	healthHTTPClient := &http.Client{Timeout: 2 * time.Second}

	// 健康检查 (不受中间件保护，或在中间件中豁免)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkHTTP(r.Context(), "clearing", clearingBaseURL, healthHTTPClient),
		}
		writeHealth(w, deps)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkHTTP(r.Context(), "clearing", clearingBaseURL, healthHTTPClient),
		}
		writeHealth(w, deps)
	})

	// Swagger UI - API 文档
	// 访问 /docs 查看交互式 API 文档，支持在线测试
	// 访问 /openapi.yaml 获取 OpenAPI 3.0 规范文件
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
    <title>Exchange Wallet API Documentation</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <style>
        body { margin: 0; padding: 0; }
        .swagger-ui .topbar { display: none; }
        .swagger-ui .info { margin: 20px 0; }
        .swagger-ui .scheme-container { background: #fafafa; padding: 15px 0; }
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
                presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
                layout: "StandaloneLayout",
                tryItOutEnabled: true,
                persistAuthorization: true,
                filter: true
            });
        };
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})

	// ========== 资产与网络 ==========
	mux.HandleFunc("/wallet/assets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		assets, err := svc.ListAssets(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(assets)
	})

	mux.HandleFunc("/wallet/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		asset := r.URL.Query().Get("asset")
		networks, err := svc.ListNetworks(r.Context(), asset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(networks)
	})

	// ========== 充值 ==========
	mux.HandleFunc("/wallet/deposit/address", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := getUserID(r)
		asset := r.URL.Query().Get("asset")
		network := r.URL.Query().Get("network")

		if asset == "" || network == "" {
			http.Error(w, "asset and network required", http.StatusBadRequest)
			return
		}

		addr, err := svc.GetDepositAddress(r.Context(), userID, asset, network)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(addr)
	})

	mux.HandleFunc("/wallet/deposits", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := getUserID(r)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		deposits, err := svc.ListDeposits(r.Context(), userID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(deposits)
	})

	// ========== 提现 ==========
	mux.HandleFunc("/wallet/withdraw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := getUserID(r)
		var req struct {
			IdempotencyKey string  `json:"idempotencyKey"`
			Asset          string  `json:"asset"`
			Network        string  `json:"network"`
			Amount         int64   `json:"amount"`
			Address        string  `json:"address"`
			Tag            string  `json:"tag"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := svc.RequestWithdraw(r.Context(), &service.WithdrawRequest{
			IdempotencyKey: req.IdempotencyKey,
			UserID:         userID,
			Asset:          req.Asset,
			Network:        req.Network,
			Amount:         req.Amount,
			Address:        req.Address,
			Tag:            req.Tag,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/wallet/withdrawals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := getUserID(r)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		withdrawals, err := svc.ListWithdrawals(r.Context(), userID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(withdrawals)
	})

	// ========== 管理接口 ==========
	mux.HandleFunc("/wallet/admin/withdrawals/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		withdrawals, err := svc.ListPendingWithdrawals(r.Context(), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(withdrawals)
	})

	mux.HandleFunc("/wallet/admin/withdraw/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		approverID := getApproverID(r)
		var req struct {
			WithdrawID int64 `json:"withdrawId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := svc.ApproveWithdraw(r.Context(), req.WithdrawID, approverID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	mux.HandleFunc("/wallet/admin/withdraw/reject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		approverID := getApproverID(r)
		var req struct {
			WithdrawID int64 `json:"withdrawId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := svc.RejectWithdraw(r.Context(), req.WithdrawID, approverID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	mux.HandleFunc("/wallet/admin/withdraw/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			WithdrawID int64  `json:"withdrawId"`
			Txid       string `json:"txid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := svc.CompleteWithdraw(r.Context(), req.WithdrawID, req.Txid); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// 中间件
	handler := authMiddleware(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: handler,
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

func getUserID(r *http.Request) int64 {
	userID, _ := strconv.ParseInt(r.Header.Get("X-User-ID"), 10, 64)
	return userID
}

func getApproverID(r *http.Request) int64 {
	approverID, _ := strconv.ParseInt(r.Header.Get("X-Approver-ID"), 10, 64)
	return approverID
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

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 跳过健康检查和文档
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/docs" || r.URL.Path == "/openapi.yaml" {
			next.ServeHTTP(w, r)
			return
		}

		// 2. 获取 Token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}

		token := parts[1]

		// 3. 解析 Token (格式: token_{uid})
		if !strings.HasPrefix(token, "token_") {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		userIDStr := strings.TrimPrefix(token, "token_")
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid token payload", http.StatusUnauthorized)
			return
		}

		// 4. 设置 Header (供后续 handler 使用)
		r.Header.Set("X-User-ID", fmt.Sprintf("%d", userID))
		r.Header.Set("X-Approver-ID", fmt.Sprintf("%d", userID)) // 假设 Approve 也是同一个人
		next.ServeHTTP(w, r)
	})
}
