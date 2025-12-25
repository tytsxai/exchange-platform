// Package service 订单更新消费者
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/exchange/common/pkg/health"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/metrics"
	"github.com/exchange/order/internal/repository"
	"github.com/redis/go-redis/v9"
)

// UpdaterConfig 配置
type UpdaterConfig struct {
	EventStream string
	Group       string
	Consumer    string
}

// OrderUpdater 消费撮合事件更新订单
type OrderUpdater struct {
	redis      *redis.Client
	orderStore OrderUpdaterStore
	tradeStore TradeStore
	clearing   ClearingUnfreezer
	metrics    *metrics.Metrics

	eventStream string
	group       string
	consumer    string

	loop health.LoopMonitor
}

const (
	defaultMaxStreamRetries = 10
)

// OrderUpdaterStore 订单更新依赖接口
type OrderUpdaterStore interface {
	UpdateOrderStatus(ctx context.Context, orderID int64, status int, executedQty, cumulativeQuoteQty, updateTimeMs int64) error
	CancelOrder(ctx context.Context, orderID int64, reason string, updateTimeMs int64) error
	RejectOrder(ctx context.Context, orderID int64, reason string, updateTimeMs int64) error
	GetOrder(ctx context.Context, orderID int64) (*repository.Order, error)
	GetSymbolConfig(ctx context.Context, symbol string) (*repository.SymbolConfig, error)
	AddOrderCumulativeQuoteQty(ctx context.Context, orderID int64, delta int64, updateTimeMs int64) error
}

// TradeStore 成交存储接口
type TradeStore interface {
	SaveTrade(ctx context.Context, trade *repository.Trade) error
}

// ClearingUnfreezer 解冻接口
type ClearingUnfreezer interface {
	UnfreezeBalance(ctx context.Context, userID int64, asset string, amount int64, idempotencyKey string) (*client.UnfreezeResponse, error)
}

// NewOrderUpdater 创建更新服务
func NewOrderUpdater(redisClient *redis.Client, orderStore OrderUpdaterStore, tradeStore TradeStore, clearing ClearingUnfreezer, metricsClient *metrics.Metrics, cfg *UpdaterConfig) *OrderUpdater {
	return &OrderUpdater{
		redis:       redisClient,
		orderStore:  orderStore,
		tradeStore:  tradeStore,
		clearing:    clearing,
		metrics:     metricsClient,
		eventStream: cfg.EventStream,
		group:       cfg.Group,
		consumer:    cfg.Consumer,
	}
}

// Start 启动消费
func (u *OrderUpdater) Start(ctx context.Context) error {
	err := u.redis.XGroupCreateMkStream(ctx, u.eventStream, u.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("create consumer group: %w", err)
	}

	u.loop.Tick()
	go u.consumeLoop(ctx)
	return nil
}

func (u *OrderUpdater) ConsumeLoopHealthy(now time.Time, maxAge time.Duration) (bool, time.Duration, string) {
	return u.loop.Healthy(now, maxAge)
}

func (u *OrderUpdater) consumeLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			u.loop.SetError(fmt.Errorf("panic: %v", r))
			log.Printf("consumeLoop panic: %v\n%s", r, string(debug.Stack()))
		}
	}()

	log.Printf("Order updater consuming %s", u.eventStream)

	pendingTicker := time.NewTicker(30 * time.Second)
	defer pendingTicker.Stop()

	if err := u.processPending(ctx); err != nil {
		u.loop.SetError(err)
		log.Printf("process pending error: %v", err)
	}

	for {
		u.loop.Tick()

		select {
		case <-ctx.Done():
			return
		case <-pendingTicker.C:
			if err := u.processPending(ctx); err != nil {
				u.loop.SetError(err)
				log.Printf("process pending error: %v", err)
			}
			continue
		default:
		}

		if err := u.consumeOnce(ctx); err != nil {
			u.loop.SetError(err)
			log.Printf("read stream error: %v", err)
		}
	}
}

func (u *OrderUpdater) consumeOnce(ctx context.Context) error {
	results, err := u.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    u.group,
		Consumer: u.consumer,
		Streams:  []string{u.eventStream, ">"},
		Count:    100,
		Block:    1000 * time.Millisecond,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}

	for _, result := range results {
		for _, msg := range result.Messages {
			if err := u.processMessage(ctx, msg); err != nil {
				if u.metrics != nil {
					u.metrics.IncStreamError(u.eventStream, u.group)
				}
				log.Printf("process event error: %v", err)
				continue
			}
			u.redis.XAck(ctx, u.eventStream, u.group, msg.ID)
		}
	}
	return nil
}

func (u *OrderUpdater) processPending(ctx context.Context) error {
	if u.metrics != nil {
		if summary, err := u.redis.XPending(ctx, u.eventStream, u.group).Result(); err == nil {
			u.metrics.SetStreamPending(u.eventStream, u.group, summary.Count)
		}
	}

	pending, err := u.redis.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: u.eventStream,
		Group:  u.group,
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
			if entry.RetryCount > defaultMaxStreamRetries {
				dlqIDs[entry.ID] = entry.RetryCount
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	claimed, err := u.redis.XClaim(ctx, &redis.XClaimArgs{
		Stream:   u.eventStream,
		Group:    u.group,
		Consumer: u.consumer,
		MinIdle:  30 * time.Second,
		Messages: ids,
	}).Result()
	if err != nil {
		return err
	}

	for _, msg := range claimed {
		if retryCount, toDLQ := dlqIDs[msg.ID]; toDLQ {
			if err := u.sendToDLQ(ctx, &msg, fmt.Sprintf("max retries exceeded: %d", retryCount)); err != nil {
				if u.metrics != nil {
					u.metrics.IncStreamError(u.eventStream, u.group)
				}
				log.Printf("send dlq error: %v", err)
				continue
			}
			if u.metrics != nil {
				u.metrics.IncStreamDLQ(u.eventStream, u.group)
			}
			u.redis.XAck(ctx, u.eventStream, u.group, msg.ID)
			continue
		}
		if err := u.processMessage(ctx, msg); err != nil {
			if u.metrics != nil {
				u.metrics.IncStreamError(u.eventStream, u.group)
			}
			log.Printf("process pending event error: %v", err)
			continue
		}
		u.redis.XAck(ctx, u.eventStream, u.group, msg.ID)
	}
	return nil
}

func (u *OrderUpdater) sendToDLQ(ctx context.Context, msg *redis.XMessage, reason string) error {
	dlqStream := u.eventStream + ":dlq"
	_, err := u.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: map[string]interface{}{
			"stream":   u.eventStream,
			"msgId":    msg.ID,
			"reason":   reason,
			"data":     msg.Values["data"],
			"tsMs":     time.Now().UnixMilli(),
			"group":    u.group,
			"consumer": u.consumer,
		},
	}).Result()
	return err
}

func (u *OrderUpdater) processMessage(ctx context.Context, msg redis.XMessage) error {
	data, ok := msg.Values["data"].(string)
	if !ok {
		return nil
	}

	var event MatchingEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	switch event.Type {
	case "ORDER_ACCEPTED":
		return u.handleOrderAccepted(ctx, &event)
	case "ORDER_REJECTED":
		return u.handleOrderRejected(ctx, &event)
	case "ORDER_PARTIALLY_FILLED":
		return u.handleOrderPartiallyFilled(ctx, &event)
	case "ORDER_FILLED":
		return u.handleOrderFilled(ctx, &event)
	case "ORDER_CANCELED":
		return u.handleOrderCanceled(ctx, &event)
	case "TRADE_CREATED":
		return u.handleTradeCreated(ctx, &event)
	default:
		return nil
	}
}

// MatchingEvent 撮合事件
type MatchingEvent struct {
	Type      string          `json:"type"`
	Symbol    string          `json:"symbol"`
	Seq       int64           `json:"seq"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// OrderAcceptedData 订单接受数据
type OrderAcceptedData struct {
	OrderID int64 `json:"OrderID"`
}

// OrderPartiallyFilledData 订单部分成交数据
type OrderPartiallyFilledData struct {
	OrderID     int64 `json:"OrderID"`
	ExecutedQty int64 `json:"ExecutedQty"`
}

// OrderFilledData 订单完全成交数据
type OrderFilledData struct {
	OrderID     int64 `json:"OrderID"`
	ExecutedQty int64 `json:"ExecutedQty"`
}

// OrderCanceledData 订单取消数据
type OrderCanceledData struct {
	OrderID   int64  `json:"OrderID"`
	UserID    int64  `json:"UserID"`
	LeavesQty int64  `json:"LeavesQty"`
	Reason    string `json:"Reason"`
}

// OrderRejectedData 订单拒绝数据
type OrderRejectedData struct {
	OrderID int64  `json:"OrderID"`
	UserID  int64  `json:"UserID"`
	Reason  string `json:"Reason"`
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
	TakerSide    int   `json:"TakerSide"`
}

func (u *OrderUpdater) handleOrderAccepted(ctx context.Context, event *MatchingEvent) error {
	var data OrderAcceptedData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal order accepted: %w", err)
	}

	cumulative, err := u.getCumulativeQuoteQty(ctx, data.OrderID)
	if err != nil {
		return err
	}

	if err := u.orderStore.UpdateOrderStatus(ctx, data.OrderID, repository.StatusNew, 0, cumulative, time.Now().UnixMilli()); err != nil {
		return err
	}
	if u.metrics != nil {
		u.metrics.IncActiveOrders()
	}
	return nil
}

func (u *OrderUpdater) handleOrderPartiallyFilled(ctx context.Context, event *MatchingEvent) error {
	var data OrderPartiallyFilledData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal order partially filled: %w", err)
	}

	cumulative, err := u.getCumulativeQuoteQty(ctx, data.OrderID)
	if err != nil {
		return err
	}

	return u.orderStore.UpdateOrderStatus(ctx, data.OrderID, repository.StatusPartiallyFilled, data.ExecutedQty, cumulative, time.Now().UnixMilli())
}

func (u *OrderUpdater) handleOrderFilled(ctx context.Context, event *MatchingEvent) error {
	var data OrderFilledData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal order filled: %w", err)
	}

	cumulative, err := u.getCumulativeQuoteQty(ctx, data.OrderID)
	if err != nil {
		return err
	}

	if err := u.orderStore.UpdateOrderStatus(ctx, data.OrderID, repository.StatusFilled, data.ExecutedQty, cumulative, time.Now().UnixMilli()); err != nil {
		return err
	}
	if u.metrics != nil {
		u.metrics.DecActiveOrders()
	}
	if err := u.unfreezeForFilled(ctx, data.OrderID); err != nil {
		return err
	}
	return nil
}

func (u *OrderUpdater) handleOrderCanceled(ctx context.Context, event *MatchingEvent) error {
	var data OrderCanceledData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal order canceled: %w", err)
	}

	if err := u.orderStore.CancelOrder(ctx, data.OrderID, data.Reason, time.Now().UnixMilli()); err != nil && err != repository.ErrOrderNotFound {
		return err
	}

	order, err := u.orderStore.GetOrder(ctx, data.OrderID)
	if err != nil {
		return err
	}
	if u.metrics != nil {
		u.metrics.DecActiveOrders()
	}

	cfg, err := u.orderStore.GetSymbolConfig(ctx, order.Symbol)
	if err != nil {
		return err
	}

	amount, asset, err := u.calculateCancelUnfreeze(order, cfg, data.LeavesQty)
	if err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}

	unfreezeKey := fmt.Sprintf("unfreeze:order:%d", order.OrderID)
	resp, err := u.clearing.UnfreezeBalance(ctx, order.UserID, asset, amount, unfreezeKey)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("unfreeze failed: %s", resp.ErrorCode)
	}

	return nil
}

func (u *OrderUpdater) handleOrderRejected(ctx context.Context, event *MatchingEvent) error {
	var data OrderRejectedData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal order rejected: %w", err)
	}

	if err := u.orderStore.RejectOrder(ctx, data.OrderID, data.Reason, time.Now().UnixMilli()); err != nil && err != repository.ErrOrderNotFound {
		return err
	}

	order, err := u.orderStore.GetOrder(ctx, data.OrderID)
	if err != nil {
		return err
	}

	cfg, err := u.orderStore.GetSymbolConfig(ctx, order.Symbol)
	if err != nil {
		return err
	}

	amount, asset, err := u.calculateRejectUnfreeze(order, cfg)
	if err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}

	unfreezeKey := fmt.Sprintf("unfreeze:order:%d:reject", order.OrderID)
	resp, err := u.clearing.UnfreezeBalance(ctx, order.UserID, asset, amount, unfreezeKey)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("unfreeze failed: %s", resp.ErrorCode)
	}

	return nil
}

func (u *OrderUpdater) handleTradeCreated(ctx context.Context, event *MatchingEvent) error {
	var data TradeData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return fmt.Errorf("unmarshal trade created: %w", err)
	}

	cfg, err := u.orderStore.GetSymbolConfig(ctx, event.Symbol)
	if err != nil {
		return err
	}

	quoteAmount := quoteQty(data.Price, data.Qty, cfg.QtyPrecision)
	timestampMs := event.Timestamp / 1e6
	if timestampMs == 0 {
		timestampMs = time.Now().UnixMilli()
	}

	trade := &repository.Trade{
		TradeID:      data.TradeID,
		Symbol:       event.Symbol,
		MakerOrderID: data.MakerOrderID,
		TakerOrderID: data.TakerOrderID,
		MakerUserID:  data.MakerUserID,
		TakerUserID:  data.TakerUserID,
		Price:        data.Price,
		Qty:          data.Qty,
		QuoteQty:     quoteAmount,
		MakerFee:     0,
		TakerFee:     0,
		FeeAsset:     cfg.QuoteAsset,
		TakerSide:    data.TakerSide,
		TimestampMs:  timestampMs,
	}

	if err := u.tradeStore.SaveTrade(ctx, trade); err != nil {
		if errors.Is(err, repository.ErrDuplicateTrade) {
			return nil
		}
		return err
	}

	updateTime := time.Now().UnixMilli()
	if err := u.orderStore.AddOrderCumulativeQuoteQty(ctx, data.MakerOrderID, quoteAmount, updateTime); err != nil {
		return err
	}
	if err := u.orderStore.AddOrderCumulativeQuoteQty(ctx, data.TakerOrderID, quoteAmount, updateTime); err != nil {
		return err
	}

	return nil
}

func (u *OrderUpdater) getCumulativeQuoteQty(ctx context.Context, orderID int64) (int64, error) {
	order, err := u.orderStore.GetOrder(ctx, orderID)
	if err != nil {
		return 0, err
	}
	return parseInt64(order.CumulativeQuoteQty, "cumulative_quote_qty")
}

func (u *OrderUpdater) calculateUnfreeze(order *repository.Order, cfg *repository.SymbolConfig, leavesQty int64) (int64, string, error) {
	if order.Side == repository.SideSell {
		return leavesQty, cfg.BaseAsset, nil
	}

	price, err := strconv.ParseInt(order.Price, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse order price: %w", err)
	}

	return quoteQty(price, leavesQty, cfg.QtyPrecision), cfg.QuoteAsset, nil
}

func (u *OrderUpdater) calculateCancelUnfreeze(order *repository.Order, cfg *repository.SymbolConfig, leavesQty int64) (int64, string, error) {
	if order.Side == repository.SideSell {
		return leavesQty, cfg.BaseAsset, nil
	}
	amount, err := u.buyUnfreezeAmount(order, cfg)
	if err != nil {
		return 0, "", err
	}
	return amount, cfg.QuoteAsset, nil
}

func (u *OrderUpdater) calculateRejectUnfreeze(order *repository.Order, cfg *repository.SymbolConfig) (int64, string, error) {
	if order.Side == repository.SideSell {
		qty, err := parseInt64(order.OrigQty, "orig_qty")
		if err != nil {
			return 0, "", err
		}
		return qty, cfg.BaseAsset, nil
	}
	amount, err := u.buyUnfreezeAmount(order, cfg)
	if err != nil {
		return 0, "", err
	}
	return amount, cfg.QuoteAsset, nil
}

func (u *OrderUpdater) buyUnfreezeAmount(order *repository.Order, cfg *repository.SymbolConfig) (int64, error) {
	total, err := totalFrozenQuote(order, cfg)
	if err != nil {
		return 0, err
	}
	spent, err := parseInt64(order.CumulativeQuoteQty, "cumulative_quote_qty")
	if err != nil {
		return 0, err
	}
	amount := total - spent
	if amount < 0 {
		amount = 0
	}
	return amount, nil
}

func (u *OrderUpdater) unfreezeForFilled(ctx context.Context, orderID int64) error {
	order, err := u.orderStore.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Side != repository.SideBuy {
		return nil
	}
	cfg, err := u.orderStore.GetSymbolConfig(ctx, order.Symbol)
	if err != nil {
		return err
	}
	amount, err := u.buyUnfreezeAmount(order, cfg)
	if err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}
	unfreezeKey := fmt.Sprintf("unfreeze:order:%d:filled", order.OrderID)
	resp, err := u.clearing.UnfreezeBalance(ctx, order.UserID, cfg.QuoteAsset, amount, unfreezeKey)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("unfreeze failed: %s", resp.ErrorCode)
	}
	return nil
}

func totalFrozenQuote(order *repository.Order, cfg *repository.SymbolConfig) (int64, error) {
	price, err := parseInt64(order.Price, "price")
	if err != nil {
		return 0, err
	}
	qty, err := parseInt64(order.OrigQty, "orig_qty")
	if err != nil {
		return 0, err
	}
	return quoteQty(price, qty, cfg.QtyPrecision), nil
}

func parseInt64(value string, field string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", field, err)
	}
	return parsed, nil
}
