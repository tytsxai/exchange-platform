// Package handler 消息处理
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/exchange/matching/internal/engine"
	"github.com/exchange/matching/internal/orderbook"
	"github.com/redis/go-redis/v9"
)

// OrderMessage 订单消息（从 Redis Stream 接收）
type OrderMessage struct {
	Type          string `json:"type"`           // NEW / CANCEL
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	UserID        int64  `json:"userId"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`           // BUY / SELL
	OrderType     string `json:"orderType"`      // LIMIT / MARKET
	TimeInForce   string `json:"timeInForce"`    // GTC / IOC / FOK / POST_ONLY
	Price         int64  `json:"price"`          // 最小单位整数
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

	inputStream  string // 输入流名称
	outputStream string // 输出流名称
	group        string // 消费者组
	consumer     string // 消费者名称
}

// Config 配置
type Config struct {
	InputStream  string
	OutputStream string
	Group        string
	Consumer     string
}

// NewHandler 创建处理器
func NewHandler(redisClient *redis.Client, cfg *Config) *Handler {
	return &Handler{
		redis:        redisClient,
		engines:      make(map[string]*engine.Engine),
		inputStream:  cfg.InputStream,
		outputStream: cfg.OutputStream,
		group:        cfg.Group,
		consumer:     cfg.Consumer,
	}
}

// Start 启动处理器
func (h *Handler) Start(ctx context.Context) error {
	// 创建消费者组
	err := h.redis.XGroupCreateMkStream(ctx, h.inputStream, h.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("create consumer group: %w", err)
	}

	// 启动消费循环
	go h.consumeLoop(ctx)

	return nil
}

func (h *Handler) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 读取消息
		results, err := h.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    h.group,
			Consumer: h.consumer,
			Streams:  []string{h.inputStream, ">"},
			Count:    100,
			Block:    1000, // 1秒超时
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			log.Printf("read stream error: %v", err)
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
		log.Printf("unmarshal message error: %v", err)
		h.ack(ctx, msg.ID)
		return
	}

	// 获取或创建引擎
	eng := h.getOrCreateEngine(orderMsg.Symbol)

	// 转换为命令
	cmd := h.toCommand(&orderMsg)

	// 提交命令
	if err := eng.Submit(cmd); err != nil {
		log.Printf("submit command error: %v", err)
	}

	h.ack(ctx, msg.ID)
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
	go h.forwardEvents(eng)

	h.engines[symbol] = eng
	return eng
}

func (h *Handler) forwardEvents(eng *engine.Engine) {
	ctx := context.Background()
	for event := range eng.Events() {
		eventMsg := &EventMessage{
			Type:      eventTypeToString(event.Type),
			Symbol:    event.Symbol,
			Seq:       event.Seq,
			Timestamp: event.Timestamp,
			Data:      event.Data,
		}

		data, err := json.Marshal(eventMsg)
		if err != nil {
			log.Printf("marshal event error: %v", err)
			continue
		}

		// 发送到输出流
		_, err = h.redis.XAdd(ctx, &redis.XAddArgs{
			Stream: h.outputStream,
			Values: map[string]interface{}{
				"data": string(data),
			},
		}).Result()

		if err != nil {
			log.Printf("send event error: %v", err)
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
	h.redis.XAck(ctx, h.inputStream, h.group, id)
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
	h.mu.RLock()
	eng, exists := h.engines[symbol]
	h.mu.RUnlock()

	if !exists {
		return nil, nil, false
	}

	bids, asks = eng.Depth(limit)
	return bids, asks, true
}
