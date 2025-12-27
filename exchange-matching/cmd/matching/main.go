package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/exchange/common/pkg/snowflake"
	"github.com/exchange/matching/internal/config"
	"github.com/exchange/matching/internal/handler"
	"github.com/exchange/matching/internal/metrics"
	"github.com/exchange/matching/internal/orderbook"
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

	// 连接 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     200,
		MinIdleConns: 20,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试 Redis 连接
	redisPingCtx, redisPingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer redisPingCancel()
	if err := redisClient.Ping(redisPingCtx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis at %s", cfg.RedisAddr)

	// 创建处理器
	h := handler.NewHandler(redisClient, &handler.Config{
		OrderStream: cfg.OrderStream,
		EventStream: cfg.EventStream,
		Group:       cfg.ConsumerGroup,
		Consumer:    cfg.ConsumerName,
	})

	// 启动处理器
	if err := h.Start(ctx); err != nil {
		log.Fatalf("Failed to start handler: %v", err)
	}
	log.Printf("Handler started, consuming from %s", cfg.OrderStream)

	// HTTP 服务（健康检查 + 深度查询）
	mux := http.NewServeMux()
	requireInternalAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Internal-Token") != cfg.InternalToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkRedis(r.Context(), redisClient),
			checkConsumeLoop(h),
		}
		writeHealth(w, deps)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkRedis(r.Context(), redisClient),
			checkConsumeLoop(h),
		}
		writeHealth(w, deps)
	})
	metricsHandler := metrics.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			metrics.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)
	depthHandler := requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/depth", depthHandler)
	mux.HandleFunc("/v1/depth", depthHandler)

	if cfg.AppEnv == "dev" || os.Getenv("ALLOW_INTERNAL_RESET") == "1" {
		resetHandler := requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			symbol := r.URL.Query().Get("symbol")
			reset := h.ResetEngines(symbol)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"reset":  reset,
				"symbol": symbol,
			})
		})
		mux.HandleFunc("/internal/reset", resetHandler)
	}

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
	redisClient.Close()
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

func checkConsumeLoop(h *handler.Handler) dependencyStatus {
	now := time.Now()
	ok, age, _ := h.ConsumeLoopHealthy(now, 45*time.Second)
	status := "ok"
	if !ok {
		status = "down"
	}
	return dependencyStatus{
		Name:    "orderStreamConsumer",
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

// 保留 orderbook 包引用
var _ = orderbook.SideBuy
