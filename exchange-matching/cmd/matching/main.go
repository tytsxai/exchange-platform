package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/exchange/matching/internal/config"
	"github.com/exchange/matching/internal/handler"
	"github.com/exchange/matching/internal/orderbook"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting %s...", cfg.ServiceName)

	// 连接 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试 Redis 连接
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis at %s", cfg.RedisAddr)

	// 创建处理器
	h := handler.NewHandler(redisClient, &handler.Config{
		InputStream:  cfg.InputStream,
		OutputStream: cfg.OutputStream,
		Group:        cfg.ConsumerGroup,
		Consumer:     cfg.ConsumerName,
	})

	// 启动处理器
	if err := h.Start(ctx); err != nil {
		log.Fatalf("Failed to start handler: %v", err)
	}
	log.Printf("Handler started, consuming from %s", cfg.InputStream)

	// HTTP 服务（健康检查 + 深度查询）
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/depth", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}
		bids, asks, ok := h.GetDepth(symbol, 20)
		if !ok {
			http.Error(w, "symbol not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"bids": bids,
			"asks": asks,
		}
		json.NewEncoder(w).Encode(resp)
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
	redisClient.Close()
	log.Println("Shutdown complete")
}

// 保留 orderbook 包引用
var _ = orderbook.SideBuy
