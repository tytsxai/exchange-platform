package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/exchange/marketdata/internal/config"
	"github.com/exchange/marketdata/internal/service"
	"github.com/exchange/marketdata/internal/ws"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

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

	// 创建行情服务
	svc := service.NewMarketDataService(redisClient, &service.Config{
		EventStream: cfg.EventStream,
		Group:       cfg.ConsumerGroup,
		Consumer:    cfg.ConsumerName,
	})

	// 启动事件消费
	if err := svc.Start(ctx); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	// 创建 WebSocket 服务器
	wsServer := ws.NewServer(svc)

	// 启动 WebSocket 服务
	go func() {
		addr := fmt.Sprintf(":%d", cfg.WSPort)
		if err := wsServer.Run(ctx, addr); err != nil && err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// HTTP REST 服务
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 盘口
	mux.HandleFunc("/v1/depth", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 20
		}

		depth := svc.GetDepth(symbol, limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(depth)
	})

	// 最近成交
	mux.HandleFunc("/v1/trades", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}

		trades := svc.GetTrades(symbol, limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trades)
	})

	// 24h 行情
	mux.HandleFunc("/v1/ticker", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")

		w.Header().Set("Content-Type", "application/json")
		if symbol != "" {
			ticker := svc.GetTicker(symbol)
			json.NewEncoder(w).Encode(ticker)
		} else {
			tickers := svc.GetAllTickers()
			json.NewEncoder(w).Encode(tickers)
		}
	})

	// WebSocket 连接数
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{
			"wsClients": wsServer.ClientCount(),
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
	cancel()
	server.Shutdown(context.Background())
	log.Println("Shutdown complete")
}
