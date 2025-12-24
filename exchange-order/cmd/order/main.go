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
	"syscall"
	"time"

	"github.com/exchange/common/pkg/snowflake"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/config"
	"github.com/exchange/order/internal/metrics"
	"github.com/exchange/order/internal/repository"
	"github.com/exchange/order/internal/service"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
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

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Printf("Connected to PostgreSQL")

	// 连接 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis")

	metricsClient := metrics.New()

	// 创建服务
	idGen := snowflakeIDGen{}
	repo := repository.NewOrderRepository(db)
	matchingClient := client.NewMatchingClient(cfg.MatchingServiceURL, cfg.InternalToken)
	clearingClient := client.NewClearingClient(cfg.ClearingBaseURL, cfg.InternalToken)
	validator := service.NewPriceValidator(repo, matchingClient, service.PriceValidatorConfig{
		Enabled:          cfg.PriceProtection.Enabled,
		DefaultLimitRate: cfg.PriceProtection.DefaultLimitRate,
	})
	svc := service.NewOrderService(repo, redisClient, idGen, cfg.OrderStream, validator, clearingClient, metricsClient)

	tradeRepo := repository.NewTradeRepository(db)
	updater := service.NewOrderUpdater(redisClient, repo, tradeRepo, clearingClient, metricsClient, &service.UpdaterConfig{
		EventStream: cfg.EventStream,
		Group:       cfg.MatchingConsumerGroup,
		Consumer:    cfg.MatchingConsumerName,
	})
	if err := updater.Start(ctx); err != nil {
		log.Fatalf("Failed to start order updater: %v", err)
	}

	// HTTP 服务
	mux := http.NewServeMux()
	healthHTTPClient := &http.Client{Timeout: 2 * time.Second}
	requireInternalAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Internal-Token") != cfg.InternalToken {
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
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "clearing", cfg.ClearingBaseURL, healthHTTPClient),
		}
		writeHealth(w, deps)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "clearing", cfg.ClearingBaseURL, healthHTTPClient),
		}
		writeHealth(w, deps)
	})

	// 交易所信息
	mux.HandleFunc("/v1/exchangeInfo", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		configs, err := svc.GetExchangeInfo(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"serverTime": time.Now().UnixMilli(),
			"symbols":    configs,
		})
	}))

	// Prometheus metrics
	metricsHandler := metricsClient.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			metricsClient.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)

	// 下单
	mux.HandleFunc("/v1/order", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateOrder(w, r, svc)
		case http.MethodDelete:
			handleCancelOrder(w, r, svc)
		case http.MethodGet:
			handleGetOrder(w, r, svc)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// 当前委托
	mux.HandleFunc("/v1/openOrders", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromHeader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		symbol := r.URL.Query().Get("symbol")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 100
		}

		orders, err := svc.ListOpenOrders(r.Context(), userID, symbol, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
	}))

	// 历史订单
	mux.HandleFunc("/v1/allOrders", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromHeader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		symbol := r.URL.Query().Get("symbol")
		startTime, _ := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
		endTime, _ := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		orders, err := svc.ListOrders(r.Context(), userID, symbol, startTime, endTime, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
	}))

	// 我的成交
	mux.HandleFunc("/v1/myTrades", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromHeader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		symbol := r.URL.Query().Get("symbol")
		if strings.TrimSpace(symbol) == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}
		startTime, _ := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
		endTime, _ := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		trades, err := tradeRepo.ListTradesByUser(r.Context(), userID, symbol, startTime, endTime, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trades)
	}))

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           mux,
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
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
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

// CreateOrderRequest 下单请求
type CreateOrderRequest struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"timeInForce"`
	Price         int64  `json:"price"`
	Quantity      int64  `json:"quantity"`
	QuoteOrderQty int64  `json:"quoteOrderQty"`
	ClientOrderID string `json:"clientOrderId"`
}

func getUserIDFromHeader(r *http.Request) (int64, error) {
	userIDStr := strings.TrimSpace(r.Header.Get("X-User-Id"))
	if userIDStr == "" {
		return 0, fmt.Errorf("X-User-Id header required")
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("invalid X-User-Id")
	}
	return userID, nil
}

func handleCreateOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, err := getUserIDFromHeader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.CreateOrder(r.Context(), &service.CreateOrderRequest{
		UserID:        userID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Type:          req.Type,
		TimeInForce:   req.TimeInForce,
		Price:         req.Price,
		Quantity:      req.Quantity,
		QuoteOrderQty: req.QuoteOrderQty,
		ClientOrderID: req.ClientOrderID,
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
	json.NewEncoder(w).Encode(resp.Order)
}

func handleCancelOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, err := getUserIDFromHeader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	symbol := r.URL.Query().Get("symbol")
	orderID, _ := strconv.ParseInt(r.URL.Query().Get("orderId"), 10, 64)
	clientOrderID := r.URL.Query().Get("clientOrderId")

	resp, err := svc.CancelOrder(r.Context(), &service.CancelOrderRequest{
		UserID:        userID,
		Symbol:        symbol,
		OrderID:       orderID,
		ClientOrderID: clientOrderID,
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
	json.NewEncoder(w).Encode(resp.Order)
}

func handleGetOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, err := getUserIDFromHeader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	orderID, _ := strconv.ParseInt(r.URL.Query().Get("orderId"), 10, 64)

	order, err := svc.GetOrder(r.Context(), userID, orderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
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
