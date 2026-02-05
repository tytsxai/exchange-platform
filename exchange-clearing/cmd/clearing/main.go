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
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/exchange/clearing/internal/config"
	"github.com/exchange/clearing/internal/metrics"
	"github.com/exchange/clearing/internal/repository"
	"github.com/exchange/clearing/internal/service"
	clearingws "github.com/exchange/clearing/internal/ws"
	commonerrors "github.com/exchange/common/pkg/errors"
	"github.com/exchange/common/pkg/health"
	commonresp "github.com/exchange/common/pkg/response"
	"github.com/exchange/common/pkg/snowflake"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

var (
	streamPending = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "redis_stream_pending",
		Help: "Number of pending messages in Redis Streams consumer groups.",
	}, []string{"stream", "group"})
	streamErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "redis_stream_handler_errors_total",
		Help: "Total number of stream handler errors.",
	}, []string{"stream", "group"})
	streamDLQ = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "redis_stream_dlq_total",
		Help: "Total number of messages moved to Redis Stream DLQ.",
	}, []string{"stream", "group"})
)

func init() {
	prometheus.MustRegister(streamPending, streamErrors, streamDLQ)
}

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

	dbPingCtx, dbPingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbPingCancel()
	if err := db.PingContext(dbPingCtx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Printf("Connected to PostgreSQL")

	// 连接 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     200,
		MinIdleConns: 20,
	})
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redisPingCtx, redisPingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer redisPingCancel()
	if err := redisClient.Ping(redisPingCtx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis")

	// 创建服务
	idGen := snowflakeIDGen{}
	svc := service.NewClearingService(db, idGen)
	svc.SetPublisher(clearingws.NewPublisher(redisClient, cfg.PrivateUserEventChannel))

	// 启动事件消费
	var eventLoop health.LoopMonitor
	eventLoop.Tick()
	if err := ensureConsumerGroup(ctx, redisClient, cfg.EventStream, cfg.ConsumerGroup); err != nil {
		log.Fatalf("Failed to create consumer group: %v", err)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				eventLoop.SetError(fmt.Errorf("panic: %v", r))
				log.Printf("consumeEvents panic: %v\n%s", r, string(debug.Stack()))
			}
		}()
		consumeEvents(ctx, redisClient, svc, cfg, &eventLoop)
	}()

	// HTTP 服务
	metricsCollector := metrics.NewDefault()
	mux := http.NewServeMux()
	healthHTTPClient := &http.Client{Timeout: 2 * time.Second}
	requireInternalAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Internal-Token") != cfg.InternalToken {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			next(w, r)
		}
	}
	metricsHandler := metricsCollector.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			metricsCollector.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkConsumeLoop(&eventLoop),
		}
		writeHealth(w, deps)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkConsumeLoop(&eventLoop),
		}
		writeHealth(w, deps)
	})

	// 获取余额
	mux.HandleFunc("/v1/account", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userIDStr := strings.TrimSpace(r.Header.Get("X-User-Id"))
		if userIDStr == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "X-User-Id header required")
			return
		}
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil || userID <= 0 {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid X-User-Id")
			return
		}

		balances, err := svc.GetBalances(r.Context(), userID)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"userId":   userID,
			"balances": toBalanceResponses(balances),
		})
	}))

	// 账本明细
	mux.HandleFunc("/v1/ledger", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userIDStr := strings.TrimSpace(r.Header.Get("X-User-Id"))
		if userIDStr == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "X-User-Id header required")
			return
		}
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil || userID <= 0 {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid X-User-Id")
			return
		}
		asset := strings.TrimSpace(r.URL.Query().Get("asset"))
		kind := strings.TrimSpace(r.URL.Query().Get("type"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		if limit > 1000 {
			limit = 1000
		}

		fetchLimit := limit
		if kind != "" {
			fetchLimit = 1000
		}

		entries, err := svc.ListLedger(r.Context(), userID, asset, fetchLimit)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		resp := toLedgerResponses(entries, kind)
		if len(resp) > limit {
			resp = resp[:limit]
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	// 冻结资金
	mux.HandleFunc("/internal/freeze", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req service.FreezeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		resp, err := svc.Freeze(r.Context(), &req)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	// 解冻资金
	mux.HandleFunc("/internal/unfreeze", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req service.UnfreezeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		resp, err := svc.Unfreeze(r.Context(), &req)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	// 扣除冻结资金（提现完成）
	mux.HandleFunc("/internal/deduct", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req service.DeductRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		resp, err := svc.Deduct(r.Context(), &req)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	// 入账（充值确认）
	mux.HandleFunc("/internal/credit", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		var req service.CreditRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		resp, err := svc.Credit(r.Context(), &req)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	handler := limitBodyMiddleware(maxBodyBytes, mux)
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

// TradeEvent 成交事件
type TradeEvent struct {
	Type      string          `json:"type"`
	Symbol    string          `json:"symbol"`
	Seq       int64           `json:"seq"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// TradeData 成交数据
type TradeData struct {
	TradeID      int64 `json:"TradeID"`
	MakerOrderID int64 `json:"MakerOrderID"`
	TakerOrderID int64 `json:"TakerOrderID"`
	MakerUserID  int64 `json:"MakerUserID"`
	TakerUserID  int64 `json:"TakerUserID"`
	Price        int64 `json:"Price"`
	Qty          int64 `json:"Qty"`
	TakerSide    int   `json:"TakerSide"` // 1=BUY, 2=SELL
}

func ensureConsumerGroup(ctx context.Context, redisClient *redis.Client, stream, group string) error {
	err := redisClient.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

func checkConsumeLoop(loop *health.LoopMonitor) dependencyStatus {
	now := time.Now()
	ok, age, _ := loop.Healthy(now, 45*time.Second)
	status := "ok"
	if !ok {
		status = "down"
	}
	return dependencyStatus{
		Name:    "eventStreamConsumer",
		Status:  status,
		Latency: age.Milliseconds(),
	}
}

func consumeEvents(ctx context.Context, redisClient *redis.Client, svc *service.ClearingService, cfg *config.Config, loop *health.LoopMonitor) {
	log.Printf("Consuming events from %s", cfg.EventStream)

	pendingTicker := time.NewTicker(30 * time.Second)
	defer pendingTicker.Stop()

	if err := processPendingEvents(ctx, redisClient, svc, cfg); err != nil {
		if loop != nil {
			loop.SetError(err)
		}
		log.Printf("Process pending error: %v", err)
	}

	for {
		if loop != nil {
			loop.Tick()
		}
		select {
		case <-ctx.Done():
			return
		case <-pendingTicker.C:
			if err := processPendingEvents(ctx, redisClient, svc, cfg); err != nil {
				if loop != nil {
					loop.SetError(err)
				}
				log.Printf("Process pending error: %v", err)
			}
			continue
		default:
		}

		results, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: cfg.ConsumerName,
			Streams:  []string{cfg.EventStream, ">"},
			Count:    100,
			Block:    1000 * time.Millisecond,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			if loop != nil {
				loop.SetError(err)
			}
			log.Printf("Read stream error: %v", err)
			continue
		}

		for _, result := range results {
			for _, msg := range result.Messages {
				processEvent(ctx, redisClient, svc, cfg, msg)
			}
		}
	}
}

func processPendingEvents(ctx context.Context, redisClient *redis.Client, svc *service.ClearingService, cfg *config.Config) error {
	if summary, err := redisClient.XPending(ctx, cfg.EventStream, cfg.ConsumerGroup).Result(); err == nil {
		streamPending.WithLabelValues(cfg.EventStream, cfg.ConsumerGroup).Set(float64(summary.Count))
	}

	pending, err := redisClient.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.EventStream,
		Group:  cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  100,
	}).Result()
	if err != nil {
		return err
	}

	var ids []string
	dlqIDs := make(map[string]int64)
	for _, entry := range pending {
		if entry.Idle >= 30*time.Second {
			ids = append(ids, entry.ID)
			if entry.RetryCount > 10 {
				dlqIDs[entry.ID] = entry.RetryCount
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	claimed, err := redisClient.XClaim(ctx, &redis.XClaimArgs{
		Stream:   cfg.EventStream,
		Group:    cfg.ConsumerGroup,
		Consumer: cfg.ConsumerName,
		MinIdle:  30 * time.Second,
		Messages: ids,
	}).Result()
	if err != nil {
		return err
	}

	for _, msg := range claimed {
		if retryCount, toDLQ := dlqIDs[msg.ID]; toDLQ {
			if err := sendToDLQ(ctx, redisClient, cfg.EventStream, cfg.ConsumerGroup, cfg.ConsumerName, msg, fmt.Sprintf("max retries exceeded: %d", retryCount)); err != nil {
				streamErrors.WithLabelValues(cfg.EventStream, cfg.ConsumerGroup).Inc()
				log.Printf("send dlq error: %v", err)
				continue
			}
			streamDLQ.WithLabelValues(cfg.EventStream, cfg.ConsumerGroup).Inc()
			redisClient.XAck(ctx, cfg.EventStream, cfg.ConsumerGroup, msg.ID)
			continue
		}
		processEvent(ctx, redisClient, svc, cfg, msg)
	}
	return nil
}

func sendToDLQ(ctx context.Context, redisClient *redis.Client, stream, group, consumer string, msg redis.XMessage, reason string) error {
	dlqStream := stream + ":dlq"
	_, err := redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: map[string]interface{}{
			"stream":   stream,
			"msgId":    msg.ID,
			"reason":   reason,
			"data":     msg.Values["data"],
			"tsMs":     time.Now().UnixMilli(),
			"group":    group,
			"consumer": consumer,
		},
	}).Result()
	return err
}

func processEvent(ctx context.Context, redisClient *redis.Client, svc *service.ClearingService, cfg *config.Config, msg redis.XMessage) {
	data, ok := msg.Values["data"].(string)
	if !ok {
		redisClient.XAck(ctx, cfg.EventStream, cfg.ConsumerGroup, msg.ID)
		return
	}

	var event TradeEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		log.Printf("Unmarshal event error: %v", err)
		redisClient.XAck(ctx, cfg.EventStream, cfg.ConsumerGroup, msg.ID)
		return
	}

	// 只处理成交事件
	if event.Type != "TRADE_CREATED" {
		redisClient.XAck(ctx, cfg.EventStream, cfg.ConsumerGroup, msg.ID)
		return
	}

	var trade TradeData
	if err := json.Unmarshal(event.Data, &trade); err != nil {
		log.Printf("Unmarshal trade error: %v", err)
		redisClient.XAck(ctx, cfg.EventStream, cfg.ConsumerGroup, msg.ID)
		return
	}

	// 解析 symbol 获取 base/quote
	baseAsset, quoteAsset := parseSymbol(event.Symbol)

	// 计算资产变动
	// TakerSide=1(BUY): taker 买入 base，卖出 quote；maker 卖出 base，买入 quote
	// TakerSide=2(SELL): taker 卖出 base，买入 quote；maker 买入 base，卖出 quote
	quoteQty := trade.Price * trade.Qty / 1e8 // 假设精度为 8 位

	var makerBaseDelta, makerQuoteDelta, takerBaseDelta, takerQuoteDelta int64
	if trade.TakerSide == 1 { // Taker BUY
		makerBaseDelta = -trade.Qty // maker 卖出 base（从冻结扣）
		makerQuoteDelta = quoteQty  // maker 收到 quote
		takerBaseDelta = trade.Qty  // taker 收到 base
		takerQuoteDelta = -quoteQty // taker 支付 quote（从冻结扣）
	} else { // Taker SELL
		makerBaseDelta = trade.Qty  // maker 收到 base
		makerQuoteDelta = -quoteQty // maker 支付 quote（从冻结扣）
		takerBaseDelta = -trade.Qty // taker 卖出 base（从冻结扣）
		takerQuoteDelta = quoteQty  // taker 收到 quote
	}

	// 手续费（简化：0.1%）
	makerFee := int64(0)        // maker 0 费率
	takerFee := quoteQty / 1000 // 0.1%

	req := &service.SettleTradeRequest{
		IdempotencyKey:  fmt.Sprintf("trade:%d", trade.TradeID),
		TradeID:         fmt.Sprintf("%d", trade.TradeID),
		Symbol:          event.Symbol,
		MakerUserID:     trade.MakerUserID,
		MakerOrderID:    fmt.Sprintf("%d", trade.MakerOrderID),
		MakerBaseDelta:  makerBaseDelta,
		MakerQuoteDelta: makerQuoteDelta,
		MakerFee:        makerFee,
		MakerFeeAsset:   quoteAsset,
		TakerUserID:     trade.TakerUserID,
		TakerOrderID:    fmt.Sprintf("%d", trade.TakerOrderID),
		TakerBaseDelta:  takerBaseDelta,
		TakerQuoteDelta: takerQuoteDelta,
		TakerFee:        takerFee,
		TakerFeeAsset:   quoteAsset,
		BaseAsset:       baseAsset,
		QuoteAsset:      quoteAsset,
	}

	_, err := svc.SettleTrade(ctx, req)
	if err != nil {
		streamErrors.WithLabelValues(cfg.EventStream, cfg.ConsumerGroup).Inc()
		log.Printf("Settle trade error: %v", err)
		// 不 ACK，等待重试
		return
	}

	redisClient.XAck(ctx, cfg.EventStream, cfg.ConsumerGroup, msg.ID)
	log.Printf("Settled trade %d", trade.TradeID)
}

func parseSymbol(symbol string) (base, quote string) {
	// 简单实现：假设 quote 是 USDT
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		return symbol[:len(symbol)-4], "USDT"
	}
	return symbol, "USDT"
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

type balanceResponse struct {
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

func toBalanceResponses(balances []*repository.Balance) []*balanceResponse {
	if len(balances) == 0 {
		return []*balanceResponse{}
	}
	resp := make([]*balanceResponse, 0, len(balances))
	for _, bal := range balances {
		if bal == nil {
			continue
		}
		resp = append(resp, &balanceResponse{
			Asset:     bal.Asset,
			Available: strconv.FormatInt(bal.Available, 10),
			Frozen:    strconv.FormatInt(bal.Frozen, 10),
		})
	}
	return resp
}

type ledgerEntryResponse struct {
	ID        int64  `json:"id"`
	Asset     string `json:"asset"`
	Type      string `json:"type"`
	Amount    string `json:"amount"`
	Balance   string `json:"balance"`
	RefID     string `json:"refId"`
	CreatedAt int64  `json:"createdAt"`
}

func toLedgerResponses(entries []*repository.LedgerEntry, kind string) []*ledgerEntryResponse {
	if len(entries) == 0 {
		return []*ledgerEntryResponse{}
	}
	filter := strings.ToUpper(strings.TrimSpace(kind))
	resp := make([]*ledgerEntryResponse, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		entryType, ok := ledgerTypeFromReason(entry.Reason)
		if !ok {
			continue
		}
		if filter != "" && entryType != filter {
			continue
		}
		amount := entry.AvailableDelta + entry.FrozenDelta
		balance := entry.AvailableAfter + entry.FrozenAfter
		resp = append(resp, &ledgerEntryResponse{
			ID:        entry.LedgerID,
			Asset:     entry.Asset,
			Type:      entryType,
			Amount:    strconv.FormatInt(amount, 10),
			Balance:   strconv.FormatInt(balance, 10),
			RefID:     entry.RefID,
			CreatedAt: entry.CreatedAt,
		})
	}
	return resp
}

func ledgerTypeFromReason(reason int) (string, bool) {
	switch reason {
	case repository.ReasonTradeSettle:
		return "TRADE", true
	case repository.ReasonDeposit:
		return "DEPOSIT", true
	case repository.ReasonWithdraw:
		return "WITHDRAW", true
	case repository.ReasonFee:
		return "FEE", true
	default:
		return "", false
	}
}
