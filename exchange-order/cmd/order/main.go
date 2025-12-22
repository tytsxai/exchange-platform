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

	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/config"
	"github.com/exchange/order/internal/metrics"
	"github.com/exchange/order/internal/repository"
	"github.com/exchange/order/internal/service"
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
	idGen := &SimpleIDGen{workerID: cfg.WorkerID}
	repo := repository.NewOrderRepository(db)
	matchingClient := client.NewMatchingClient(cfg.MatchingServiceURL)
	clearingClient := client.NewClearingClient(cfg.ClearingBaseURL)
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
	mux.HandleFunc("/v1/exchangeInfo", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// Prometheus metrics
	mux.Handle("/metrics", metricsClient.Handler())

	// 下单
	mux.HandleFunc("/v1/order", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// 当前委托
	mux.HandleFunc("/v1/openOrders", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
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
	})

	// 历史订单
	mux.HandleFunc("/v1/allOrders", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
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
	cancel()
	server.Shutdown(context.Background())
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

func handleCreateOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	if userID == 0 {
		http.Error(w, "userId required", http.StatusBadRequest)
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
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
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
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	orderID, _ := strconv.ParseInt(r.URL.Query().Get("orderId"), 10, 64)

	order, err := svc.GetOrder(r.Context(), userID, orderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}
