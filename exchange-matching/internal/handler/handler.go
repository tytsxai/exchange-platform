// Package handler 消息处理
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/exchange/common/pkg/health"
	"github.com/exchange/common/pkg/logger"
	"github.com/exchange/matching/internal/engine"
	"github.com/exchange/matching/internal/metrics"
	"github.com/exchange/matching/internal/orderbook"
	"github.com/exchange/matching/internal/types"
	"github.com/redis/go-redis/v9"
)

// OrderLoader 订单加载器接口（用于启动时恢复订单簿）
type OrderLoader interface {
	// LoadOpenOrders 加载指定 symbol 的所有 OPEN 状态订单
	// 返回按 created_at 升序排列的订单列表
	LoadOpenOrders(ctx context.Context, symbol string) ([]*OpenOrder, error)
	// ListActiveSymbols 列出所有有活跃订单的交易对
	ListActiveSymbols(ctx context.Context) ([]string, error)
}

// OpenOrder 启动恢复用的挂单快照（来自数据库）
//
// 为避免 handler <-> engine 互相依赖，这里使用 types.OpenOrder 的别名。
type OpenOrder = types.OpenOrder

// OrderMessage 订单消息（从 Redis Stream 接收）
type OrderMessage struct {
	Type          string `json:"type"` // NEW / CANCEL
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	UserID        int64  `json:"userId"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`        // BUY / SELL
	OrderType     string `json:"orderType"`   // LIMIT / MARKET
	TimeInForce   string `json:"timeInForce"` // GTC / IOC / FOK / POST_ONLY
	Price         int64  `json:"price"`       // 最小单位整数
	Qty           int64  `json:"qty"`
}

// EventMessage 事件消息（发送到 Redis Stream）
type EventMessage struct {
	Type      string      `json:"type"`
	Symbol    string      `json:"symbol"`
	Seq       int64       `json:"seq"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Handler 消息处理器
type Handler struct {
	redis   *redis.Client
	engines map[string]*engine.Engine
	mu      sync.RWMutex
	log     *logger.Logger

	orderStream string // 输入流名称
	eventStream string // 输出流名称
	group       string // 消费者组
	consumer    string // 消费者名称
	dedupeTTL   time.Duration

	orderLoader  OrderLoader
	recoveryDone chan struct{}
	ctxMu        sync.RWMutex
	ctx          context.Context

	forwardWg sync.WaitGroup // 跟踪 forwardEvents goroutine
	loop      health.LoopMonitor
}

const (
	defaultMaxStreamRetries = 10
	defaultClaimMinIdle     = 30 * time.Second
)

// Config 配置
type Config struct {
	OrderStream string
	EventStream string
	Group       string
	Consumer    string
	DedupeTTL   time.Duration
	OrderLoader OrderLoader
	Logger      *logger.Logger
}

// NewHandler 创建处理器
func NewHandler(redisClient *redis.Client, cfg *Config) *Handler {
	dedupeTTL := cfg.DedupeTTL
	if dedupeTTL <= 0 {
		dedupeTTL = 24 * time.Hour
	}
	log := cfg.Logger
	if log == nil {
		log = logger.New("matching", nil)
	}
	return &Handler{
		redis:        redisClient,
		engines:      make(map[string]*engine.Engine),
		log:          log,
		orderStream:  cfg.OrderStream,
		eventStream:  cfg.EventStream,
		group:        cfg.Group,
		consumer:     cfg.Consumer,
		dedupeTTL:    dedupeTTL,
		orderLoader:  cfg.OrderLoader,
		recoveryDone: make(chan struct{}),
	}
}

// Start 启动处理器
func (h *Handler) Start(ctx context.Context) error {
	// 创建消费者组
	err := h.redis.XGroupCreateMkStream(ctx, h.orderStream, h.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("create consumer group: %w", err)
	}

	h.ctxMu.Lock()
	h.ctx = ctx
	h.ctxMu.Unlock()

	// 恢复订单簿（在开始消费新消息之前）
	h.log.Info("recovering order books from database")
	if err := h.recoverOrderBooks(ctx); err != nil {
		h.log.WithError(err).Warn("recover order books warning")
		// 不返回错误，允许服务继续启动
	}
	close(h.recoveryDone)
	h.log.Info("order book recovery completed")

	h.loop.Tick()

	// 启动消费循环
	go h.consumeLoop(ctx)

	return nil
}

func (h *Handler) recoverOrderBooks(ctx context.Context) error {
	if h.orderLoader == nil {
		return nil
	}

	symbols, err := h.orderLoader.ListActiveSymbols(ctx)
	if err != nil {
		return fmt.Errorf("list active symbols: %w", err)
	}

	for _, symbol := range symbols {
		if err := h.recoverSymbol(ctx, symbol); err != nil {
			h.log.WithError(err).WithField("symbol", symbol).Warn("recover symbol error")
			// 继续恢复其他 symbol，不中断
		}
	}
	return nil
}

func (h *Handler) recoverSymbol(ctx context.Context, symbol string) error {
	orders, err := h.orderLoader.LoadOpenOrders(ctx, symbol)
	if err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}

	eng := h.getOrCreateEngine(symbol)
	for _, order := range orders {
		if order == nil {
			continue
		}
		// 直接添加到订单簿，不经过撮合
		if err := eng.AddOrderDirect(order); err != nil {
			h.log.WithError(err).Infof("add order direct warning", map[string]interface{}{
				"symbol": symbol, "orderID": order.OrderID,
			})
		}
	}

	h.log.Infof("recovered orders", map[string]interface{}{
		"symbol": symbol, "count": len(orders),
	})
	return nil
}

func (h *Handler) ConsumeLoopHealthy(now time.Time, maxAge time.Duration) (bool, time.Duration, string) {
	return h.loop.Healthy(now, maxAge)
}

func (h *Handler) consumeLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			h.loop.SetError(fmt.Errorf("panic: %v", r))
			h.log.Errorf("consumeLoop panic", map[string]interface{}{
				"panic": r, "stack": string(debug.Stack()),
			})
		}
	}()

	pendingTicker := time.NewTicker(30 * time.Second)
	defer pendingTicker.Stop()

	if err := h.processPending(ctx); err != nil {
		h.loop.SetError(err)
		h.log.WithError(err).Warn("process pending error")
	}

	for {
		h.loop.Tick()

		select {
		case <-ctx.Done():
			return
		case <-pendingTicker.C:
			if err := h.processPending(ctx); err != nil {
				h.loop.SetError(err)
				h.log.WithError(err).Warn("process pending error")
			}
			continue
		default:
		}

		// 读取消息
		results, err := h.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    h.group,
			Consumer: h.consumer,
			Streams:  []string{h.orderStream, ">"},
			Count:    100,
			Block:    1000 * time.Millisecond,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			h.loop.SetError(err)
			h.log.WithError(err).Warn("read stream error")
			continue
		}

		for _, result := range results {
			for _, msg := range result.Messages {
				h.processMessage(ctx, msg)
			}
		}
	}
}

func (h *Handler) processMessage(ctx context.Context, msg redis.XMessage) {
	data, ok := msg.Values["data"].(string)
	if !ok {
		h.ack(ctx, msg.ID)
		return
	}

	var orderMsg OrderMessage
	if err := json.Unmarshal([]byte(data), &orderMsg); err != nil {
		h.log.WithError(err).Warn("unmarshal message error")
		h.ack(ctx, msg.ID)
		return
	}

	if !h.shouldProcess(ctx, &orderMsg) {
		h.ack(ctx, msg.ID)
		return
	}

	// 获取或创建引擎
	eng := h.getOrCreateEngine(orderMsg.Symbol)

	// 转换为命令
	cmd := h.toCommand(&orderMsg)

	// 提交命令
	if err := eng.Submit(cmd); err != nil {
		metrics.IncStreamError(h.orderStream, h.group)
		h.log.WithError(err).Warn("submit command error")
		return
	}

	h.ack(ctx, msg.ID)
}

func (h *Handler) shouldProcess(ctx context.Context, msg *OrderMessage) bool {
	if h.dedupeTTL <= 0 || msg == nil || msg.OrderID <= 0 {
		return true
	}
	key := fmt.Sprintf("dedupe:%s:%d", strings.ToLower(msg.Type), msg.OrderID)
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ok, err := h.redis.SetNX(timeoutCtx, key, "1", h.dedupeTTL).Result()
	if err != nil {
		h.log.WithError(err).Warn("dedupe check error")
		return true
	}
	return ok
}

func (h *Handler) processPending(ctx context.Context) error {
	if summary, err := h.redis.XPending(ctx, h.orderStream, h.group).Result(); err == nil {
		metrics.SetStreamPending(h.orderStream, h.group, summary.Count)
	}

	pending, err := h.redis.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: h.orderStream,
		Group:  h.group,
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
		if entry.Idle >= defaultClaimMinIdle {
			ids = append(ids, entry.ID)
			if entry.RetryCount > defaultMaxStreamRetries {
				dlqIDs[entry.ID] = entry.RetryCount
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	claimed, err := h.redis.XClaim(ctx, &redis.XClaimArgs{
		Stream:   h.orderStream,
		Group:    h.group,
		Consumer: h.consumer,
		MinIdle:  defaultClaimMinIdle,
		Messages: ids,
	}).Result()
	if err != nil {
		return err
	}

	for _, msg := range claimed {
		if retryCount, toDLQ := dlqIDs[msg.ID]; toDLQ {
			if err := h.sendToDLQ(ctx, &msg, fmt.Sprintf("max retries exceeded: %d", retryCount)); err != nil {
				metrics.IncStreamError(h.orderStream, h.group)
				h.log.WithError(err).Warn("send dlq error")
				continue
			}
			metrics.IncStreamDLQ(h.orderStream, h.group)
			h.ack(ctx, msg.ID)
			continue
		}
		h.processMessage(ctx, msg)
	}
	return nil
}

func (h *Handler) sendToDLQ(ctx context.Context, msg *redis.XMessage, reason string) error {
	dlqStream := h.orderStream + ":dlq"
	_, err := h.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: map[string]interface{}{
			"stream":   h.orderStream,
			"msgId":    msg.ID,
			"reason":   reason,
			"data":     msg.Values["data"],
			"tsMs":     time.Now().UnixMilli(),
			"group":    h.group,
			"consumer": h.consumer,
		},
	}).Result()
	return err
}

func (h *Handler) getOrCreateEngine(symbol string) *engine.Engine {
	h.mu.RLock()
	eng, exists := h.engines[symbol]
	h.mu.RUnlock()

	if exists {
		return eng
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// 双重检查
	if eng, exists = h.engines[symbol]; exists {
		return eng
	}

	eng = engine.NewEngine(symbol, 10000, 10000)
	eng.Start()

	// 启动事件转发
	h.ctxMu.RLock()
	evtCtx := h.ctx
	h.ctxMu.RUnlock()
	if evtCtx == nil {
		evtCtx = context.Background()
	}
	h.forwardWg.Add(1)
	go h.forwardEvents(evtCtx, eng)

	h.engines[symbol] = eng
	return eng
}

func (h *Handler) forwardEvents(ctx context.Context, eng *engine.Engine) {
	defer h.forwardWg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-eng.Done():
			return
		case event := <-eng.Events():
			if event == nil {
				continue
			}
			eventMsg := &EventMessage{
				Type:      eventTypeToString(event.Type),
				Symbol:    event.Symbol,
				Seq:       event.Seq,
				Timestamp: event.Timestamp,
				Data:      event.Data,
			}

			data, err := json.Marshal(eventMsg)
			if err != nil {
				h.log.WithError(err).Warn("marshal event error")
				continue
			}

			if err := h.publishEvent(ctx, data); err != nil && ctx.Err() == nil {
				h.log.WithError(err).Warn("send event error")
			}
		}
	}
}

func (h *Handler) publishEvent(ctx context.Context, payload []byte) error {
	backoff := 200 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sendCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := h.redis.XAdd(sendCtx, &redis.XAddArgs{
			Stream: h.eventStream,
			Values: map[string]interface{}{
				"data": string(payload),
			},
		}).Result()
		cancel()
		if err == nil {
			return nil
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}

func (h *Handler) toCommand(msg *OrderMessage) *engine.Command {
	cmd := &engine.Command{
		OrderID:       msg.OrderID,
		ClientOrderID: msg.ClientOrderID,
		UserID:        msg.UserID,
		Symbol:        msg.Symbol,
	}

	switch msg.Type {
	case "NEW":
		cmd.Type = engine.CmdNewOrder
	case "CANCEL":
		cmd.Type = engine.CmdCancelOrder
		return cmd
	default:
		cmd.Type = engine.CmdNewOrder
	}

	// Side
	switch msg.Side {
	case "BUY":
		cmd.Side = orderbook.SideBuy
	case "SELL":
		cmd.Side = orderbook.SideSell
	}

	// OrderType
	switch msg.OrderType {
	case "LIMIT":
		cmd.OrderType = 1
	case "MARKET":
		cmd.OrderType = 2
	default:
		cmd.OrderType = 1
	}

	// TimeInForce
	switch msg.TimeInForce {
	case "GTC":
		cmd.TimeInForce = 1
	case "IOC":
		cmd.TimeInForce = 2
	case "FOK":
		cmd.TimeInForce = 3
	case "POST_ONLY":
		cmd.TimeInForce = 4
	default:
		cmd.TimeInForce = 1
	}

	cmd.Price = msg.Price
	cmd.Qty = msg.Qty

	return cmd
}

func (h *Handler) ack(ctx context.Context, id string) {
	if err := h.redis.XAck(ctx, h.orderStream, h.group, id).Err(); err != nil {
		h.log.WithError(err).WithField("msgId", id).Warn("ack message error")
	}
}

func eventTypeToString(t engine.EventType) string {
	switch t {
	case engine.EventOrderAccepted:
		return "ORDER_ACCEPTED"
	case engine.EventOrderRejected:
		return "ORDER_REJECTED"
	case engine.EventOrderCanceled:
		return "ORDER_CANCELED"
	case engine.EventTradeCreated:
		return "TRADE_CREATED"
	case engine.EventOrderFilled:
		return "ORDER_FILLED"
	case engine.EventOrderPartiallyFilled:
		return "ORDER_PARTIALLY_FILLED"
	default:
		return "UNKNOWN"
	}
}

// GetDepth 获取深度
func (h *Handler) GetDepth(symbol string, limit int) (bids, asks []orderbook.PriceQty, ok bool) {
	if symbol == "" {
		return nil, nil, false
	}
	eng := h.getOrCreateEngine(symbol)
	bids, asks = eng.Depth(limit)
	return bids, asks, true
}

func (h *Handler) ResetEngines(symbol string) int {
	h.mu.Lock()

	resetOne := func(key string, eng *engine.Engine) {
		eng.Stop()
		delete(h.engines, key)
	}

	if symbol != "" {
		eng, ok := h.engines[symbol]
		if !ok {
			h.mu.Unlock()
			return 0
		}
		resetOne(symbol, eng)
		h.mu.Unlock()
		// 等待对应的 forwardEvents goroutine 退出
		h.forwardWg.Wait()
		return 1
	}

	count := 0
	for key, eng := range h.engines {
		resetOne(key, eng)
		count++
	}
	h.mu.Unlock()
	// 等待所有 forwardEvents goroutine 退出
	h.forwardWg.Wait()
	return count
}

// Stop 优雅关闭处理器
func (h *Handler) Stop() {
	h.log.Info("stopping handler")
	h.ResetEngines("")
	h.log.Info("handler stopped")
}
