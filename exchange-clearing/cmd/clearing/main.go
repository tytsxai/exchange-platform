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

	"github.com/exchange/clearing/internal/config"
	"github.com/exchange/clearing/internal/service"
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
	svc := service.NewClearingService(db, idGen)

	// 启动事件消费
	go consumeEvents(ctx, redisClient, svc, cfg)

	// HTTP 服务
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 获取余额
	mux.HandleFunc("/v1/account", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
		if userID == 0 {
			http.Error(w, "userId required", http.StatusBadRequest)
			return
		}

		balances, err := svc.GetBalances(r.Context(), userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"userId":   userID,
			"balances": balances,
		})
	})

	// 冻结资金
	mux.HandleFunc("/internal/freeze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req service.FreezeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := svc.Freeze(r.Context(), &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 解冻资金
	mux.HandleFunc("/internal/unfreeze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req service.UnfreezeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := svc.Unfreeze(r.Context(), &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 扣除资金（提现完成）
	mux.HandleFunc("/internal/deduct", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req service.DeductRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := svc.Deduct(r.Context(), &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
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
	log.Println("Shutdown complete")
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

func consumeEvents(ctx context.Context, redisClient *redis.Client, svc *service.ClearingService, cfg *config.Config) {
	// 创建消费者组
	err := redisClient.XGroupCreateMkStream(ctx, cfg.EventStream, cfg.ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Printf("Failed to create consumer group: %v", err)
	}

	log.Printf("Consuming events from %s", cfg.EventStream)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		results, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: cfg.ConsumerName,
			Streams:  []string{cfg.EventStream, ">"},
			Count:    100,
			Block:    1000,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
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
		makerBaseDelta = -trade.Qty   // maker 卖出 base（从冻结扣）
		makerQuoteDelta = quoteQty    // maker 收到 quote
		takerBaseDelta = trade.Qty    // taker 收到 base
		takerQuoteDelta = -quoteQty   // taker 支付 quote（从冻结扣）
	} else { // Taker SELL
		makerBaseDelta = trade.Qty    // maker 收到 base
		makerQuoteDelta = -quoteQty   // maker 支付 quote（从冻结扣）
		takerBaseDelta = -trade.Qty   // taker 卖出 base（从冻结扣）
		takerQuoteDelta = quoteQty    // taker 收到 quote
	}

	// 手续费（简化：0.1%）
	makerFee := int64(0) // maker 0 费率
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
