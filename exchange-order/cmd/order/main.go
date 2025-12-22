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

	"github.com/exchange/order/internal/config"
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

	// 创建服务
	idGen := &SimpleIDGen{workerID: cfg.WorkerID}
	repo := repository.NewOrderRepository(db)
	svc := service.NewOrderService(repo, redisClient, idGen, cfg.OrderStream)

	// HTTP 服务
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
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
