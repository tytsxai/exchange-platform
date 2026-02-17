package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	commonauth "github.com/exchange/common/pkg/auth"
	commonerrors "github.com/exchange/common/pkg/errors"
	commonresp "github.com/exchange/common/pkg/response"
	"github.com/exchange/common/pkg/snowflake"
	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/config"
	"github.com/exchange/wallet/internal/repository"
	"github.com/exchange/wallet/internal/service"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	repo := repository.NewWalletRepository(db)
	clearingBaseURL := cfg.ClearingServiceURL
	clearingCli := client.NewClearingClient(clearingBaseURL, cfg.InternalToken)
	tronCli := client.NewTronClient(cfg.TronNodeURL, cfg.TronGridAPIKey)
	svc := service.NewWalletService(repo, idGen, clearingCli, tronCli)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	var scanner *service.DepositScanner
	if cfg.DepositScannerEnabled {
		interval := time.Duration(cfg.DepositScannerIntervalSecs) * time.Second
		scanner = service.NewDepositScanner(svc, interval, cfg.DepositScannerMaxAddresses)
		go scanner.Start(bgCtx)
	}

	// HTTP 服务
	mux := http.NewServeMux()
	healthHTTPClient := &http.Client{Timeout: 2 * time.Second}

	// 健康检查 (不受中间件保护，或在中间件中豁免)
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkHTTP(r.Context(), "clearing", clearingBaseURL, healthHTTPClient),
		}
		if scanner != nil {
			deps = append(deps, checkScannerLoop(scanner))
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
			checkHTTP(r.Context(), "clearing", clearingBaseURL, healthHTTPClient),
		}
		if scanner != nil {
			deps = append(deps, checkScannerLoop(scanner))
		}
		writeHealth(w, deps)
	})

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
	} else {
		mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
		mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	// ========== 资产与网络 ==========
	mux.HandleFunc("/wallet/assets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}
		assets, err := svc.ListAssets(r.Context())
		if err != nil {
			writeInternalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(assets)
	})

	mux.HandleFunc("/wallet/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}
		asset := r.URL.Query().Get("asset")
		networks, err := svc.ListNetworks(r.Context(), asset)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(networks)
	})

	// ========== 充值 ==========
	mux.HandleFunc("/wallet/deposit/address", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		userID := getUserID(r)
		asset := r.URL.Query().Get("asset")
		network := r.URL.Query().Get("network")

		if asset == "" || network == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "asset and network required")
			return
		}

		addr, err := svc.GetDepositAddress(r.Context(), userID, asset, network)
		if err != nil {
			switch strings.ToLower(err.Error()) {
			case "network not found":
				commonresp.WriteErrorCode(w, r, commonerrors.CodeNetworkNotFound, "network not found")
			case "deposit disabled":
				commonresp.WriteErrorCode(w, r, commonerrors.CodeDepositDisabled, "deposit disabled")
			default:
				writeInternalError(w, err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(addr)
	})

	mux.HandleFunc("/wallet/deposits", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		userID := getUserID(r)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		deposits, err := svc.ListDeposits(r.Context(), userID, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(deposits)
	})

	// ========== 提现 ==========
	mux.HandleFunc("/wallet/withdraw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		userID := getUserID(r)
		var req struct {
			IdempotencyKey string `json:"idempotencyKey"`
			Asset          string `json:"asset"`
			Network        string `json:"network"`
			Amount         int64  `json:"amount"`
			Address        string `json:"address"`
			Tag            string `json:"tag"`
		}
		if !decodeJSON(w, r, &req) {
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
			writeInternalError(w, err)
			return
		}

		if resp.ErrorCode != "" {
			commonresp.WriteErrorCode(w, r, commonerrors.Code(resp.ErrorCode), "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/wallet/withdrawals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		userID := getUserID(r)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		withdrawals, err := svc.ListWithdrawals(r.Context(), userID, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(withdrawals)
	})

	// ========== 管理接口 ==========
	mux.HandleFunc("/wallet/admin/withdrawals/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		withdrawals, err := svc.ListPendingWithdrawals(r.Context(), limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(withdrawals)
	})

	mux.HandleFunc("/wallet/admin/withdraw/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		approverID := getApproverID(r)
		var req struct {
			WithdrawID int64 `json:"withdrawId"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := svc.ApproveWithdraw(r.Context(), req.WithdrawID, approverID); err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidWithdrawRequest):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "invalid withdraw request")
			case errors.Is(err, service.ErrInvalidWithdrawState):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid withdraw state")
			case strings.Contains(strings.ToLower(err.Error()), "not found"):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeNotFound, "withdraw not found")
			default:
				writeInternalError(w, err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	mux.HandleFunc("/wallet/admin/withdraw/reject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		approverID := getApproverID(r)
		var req struct {
			WithdrawID int64 `json:"withdrawId"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := svc.RejectWithdraw(r.Context(), req.WithdrawID, approverID); err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidWithdrawRequest):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "invalid withdraw request")
			case errors.Is(err, service.ErrInvalidWithdrawState):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid withdraw state")
			case strings.Contains(strings.ToLower(err.Error()), "not found"):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeNotFound, "withdraw not found")
			default:
				writeInternalError(w, err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	mux.HandleFunc("/wallet/admin/withdraw/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req struct {
			WithdrawID int64  `json:"withdrawId"`
			Txid       string `json:"txid"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := svc.CompleteWithdraw(r.Context(), req.WithdrawID, req.Txid); err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidWithdrawRequest):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "invalid withdraw request")
			case errors.Is(err, service.ErrInvalidWithdrawState):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid withdraw state")
			case strings.Contains(strings.ToLower(err.Error()), "not found"):
				commonresp.WriteErrorCode(w, r, commonerrors.CodeNotFound, "withdraw not found")
			default:
				writeInternalError(w, err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// 中间件
	handler := authMiddleware(tokenManager, mux)
	handler = adminTokenMiddleware(cfg.AdminToken, handler)
	handler = limitBodyMiddleware(maxBodyBytes, handler)
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
	bgCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("Shutdown complete")
}

type snowflakeIDGen struct{}

func (g snowflakeIDGen) NextID() int64 {
	return snowflake.MustNextID()
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

func checkScannerLoop(scanner *service.DepositScanner) dependencyStatus {
	if scanner == nil {
		return dependencyStatus{Name: "depositScanner", Status: "ok", Latency: 0}
	}
	maxAge := 2 * scanner.Interval()
	if maxAge <= 0 {
		maxAge = 30 * time.Second
	}
	ok, age, _ := scanner.Healthy(time.Now(), maxAge)
	status := "ok"
	if !ok {
		status = "down"
	}
	return dependencyStatus{
		Name:    "depositScanner",
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

func authMiddleware(tokenManager *commonauth.TokenManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 跳过健康检查和文档
		if r.URL.Path == "/live" || r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/docs" || r.URL.Path == "/openapi.yaml" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// 2. 获取 Token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "authorization required")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "invalid authorization format")
			return
		}

		token := parts[1]

		userID, err := tokenManager.Verify(token)
		if err != nil {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "invalid token")
			return
		}

		// 4. 设置 Header (供后续 handler 使用)
		r.Header.Set("X-User-ID", fmt.Sprintf("%d", userID))
		r.Header.Set("X-Approver-ID", fmt.Sprintf("%d", userID)) // 假设 Approve 也是同一个人
		next.ServeHTTP(w, r)
	})
}

func adminTokenMiddleware(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/wallet/admin") {
			if r.Header.Get("X-Admin-Token") != adminToken {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
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
