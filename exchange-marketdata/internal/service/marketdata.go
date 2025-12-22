// Package service 行情服务
package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// MarketDataService 行情服务
type MarketDataService struct {
	redis       RedisClient
	eventStream string
	group       string
	consumer    string

	// 内存盘口
	depths map[string]*Depth
	mu     sync.RWMutex

	// 最近成交
	trades map[string][]*Trade

	// 24h ticker
	tickers map[string]*Ticker

	// 订阅者
	subscribers map[string][]chan *Event
	subMu       sync.RWMutex
}

// Depth 盘口
type Depth struct {
	Symbol       string       `json:"symbol"`
	Bids         []PriceLevel `json:"bids"`
	Asks         []PriceLevel `json:"asks"`
	LastUpdateID int64        `json:"lastUpdateId"`
	TimestampMs  int64        `json:"timestampMs"`
}

// PriceLevel 价格档位
type PriceLevel struct {
	Price int64 `json:"price"`
	Qty   int64 `json:"qty"`
}

// Trade 成交
type Trade struct {
	TradeID     int64  `json:"tradeId"`
	Symbol      string `json:"symbol"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	TakerSide   int    `json:"takerSide"`
	TimestampMs int64  `json:"timestampMs"`
}

// Ticker 24h 行情
type Ticker struct {
	Symbol             string `json:"symbol"`
	LastPrice          int64  `json:"lastPrice"`
	PriceChange        int64  `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	HighPrice          int64  `json:"highPrice"`
	LowPrice           int64  `json:"lowPrice"`
	Volume             int64  `json:"volume"`
	QuoteVolume        int64  `json:"quoteVolume"`
	OpenPrice          int64  `json:"openPrice"`
	TradeCount         int64  `json:"tradeCount"`
	OpenTimeMs         int64  `json:"openTimeMs"`
	CloseTimeMs        int64  `json:"closeTimeMs"`
}

// Event 推送事件
type Event struct {
	Channel     string      `json:"channel"`
	Seq         int64       `json:"seq"`
	TimestampMs int64       `json:"timestampMs"`
	Data        interface{} `json:"data"`
}

// Config 配置
type Config struct {
	EventStream string
	Group       string
	Consumer    string
}

// NewMarketDataService 创建行情服务
func NewMarketDataService(redisClient RedisClient, cfg *Config) *MarketDataService {
	return &MarketDataService{
		redis:       redisClient,
		eventStream: cfg.EventStream,
		group:       cfg.Group,
		consumer:    cfg.Consumer,
		depths:      make(map[string]*Depth),
		trades:      make(map[string][]*Trade),
		tickers:     make(map[string]*Ticker),
		subscribers: make(map[string][]chan *Event),
	}
}

// Start 启动服务
func (s *MarketDataService) Start(ctx context.Context) error {
	// 创建消费者组
	err := s.redis.XGroupCreateMkStream(ctx, s.eventStream, s.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}

	go s.consumeEvents(ctx)
	return nil
}

// GetDepth 获取盘口
func (s *MarketDataService) GetDepth(symbol string, limit int) *Depth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	depth, ok := s.depths[symbol]
	if !ok {
		return &Depth{Symbol: symbol, Bids: []PriceLevel{}, Asks: []PriceLevel{}}
	}

	// 限制档位数
	result := &Depth{
		Symbol:       depth.Symbol,
		LastUpdateID: depth.LastUpdateID,
		TimestampMs:  depth.TimestampMs,
	}

	if limit <= 0 || limit > len(depth.Bids) {
		result.Bids = depth.Bids
	} else {
		result.Bids = depth.Bids[:limit]
	}

	if limit <= 0 || limit > len(depth.Asks) {
		result.Asks = depth.Asks
	} else {
		result.Asks = depth.Asks[:limit]
	}

	return result
}

// GetTrades 获取最近成交
func (s *MarketDataService) GetTrades(symbol string, limit int) []*Trade {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trades, ok := s.trades[symbol]
	if !ok {
		return []*Trade{}
	}

	if limit <= 0 || limit > len(trades) {
		return trades
	}
	return trades[len(trades)-limit:]
}

// GetTicker 获取 24h 行情
func (s *MarketDataService) GetTicker(symbol string) *Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ticker, ok := s.tickers[symbol]
	if !ok {
		return &Ticker{Symbol: symbol}
	}
	return ticker
}

// GetAllTickers 获取所有 ticker
func (s *MarketDataService) GetAllTickers() []*Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Ticker, 0, len(s.tickers))
	for _, t := range s.tickers {
		result = append(result, t)
	}
	return result
}

// Subscribe 订阅频道
func (s *MarketDataService) Subscribe(channel string) chan *Event {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	ch := make(chan *Event, 100)
	s.subscribers[channel] = append(s.subscribers[channel], ch)
	return ch
}

// Unsubscribe 取消订阅
func (s *MarketDataService) Unsubscribe(channel string, ch chan *Event) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	subs := s.subscribers[channel]
	for i, sub := range subs {
		if sub == ch {
			s.subscribers[channel] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
}

func (s *MarketDataService) consumeEvents(ctx context.Context) {
	log.Printf("Consuming events from %s", s.eventStream)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		results, err := s.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.group,
			Consumer: s.consumer,
			Streams:  []string{s.eventStream, ">"},
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
				s.processEvent(ctx, msg)
			}
		}
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

// OrderAcceptedData 订单接受数据
type OrderAcceptedData struct {
	OrderID int64 `json:"OrderID"`
	UserID  int64 `json:"UserID"`
	Side    int   `json:"Side"`
	Price   int64 `json:"Price"`
	Qty     int64 `json:"Qty"`
}

// OrderCanceledData 订单取消数据
type OrderCanceledData struct {
	OrderID   int64 `json:"OrderID"`
	UserID    int64 `json:"UserID"`
	LeavesQty int64 `json:"LeavesQty"`
}

func (s *MarketDataService) processEvent(ctx context.Context, msg redis.XMessage) {
	data, ok := msg.Values["data"].(string)
	if !ok {
		s.redis.XAck(ctx, s.eventStream, s.group, msg.ID)
		return
	}

	var event MatchingEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		log.Printf("Unmarshal event error: %v", err)
		s.redis.XAck(ctx, s.eventStream, s.group, msg.ID)
		return
	}

	switch event.Type {
	case "TRADE_CREATED":
		s.handleTradeCreated(event)
	case "ORDER_ACCEPTED":
		s.handleOrderAccepted(event)
	case "ORDER_CANCELED", "ORDER_FILLED":
		s.handleOrderRemoved(event)
	}

	s.redis.XAck(ctx, s.eventStream, s.group, msg.ID)
}

func (s *MarketDataService) handleTradeCreated(event MatchingEvent) {
	var trade TradeData
	if err := json.Unmarshal(event.Data, &trade); err != nil {
		return
	}

	now := time.Now().UnixMilli()

	// 更新最近成交
	s.mu.Lock()
	t := &Trade{
		TradeID:     trade.TradeID,
		Symbol:      event.Symbol,
		Price:       trade.Price,
		Qty:         trade.Qty,
		TakerSide:   trade.TakerSide,
		TimestampMs: now,
	}

	trades := s.trades[event.Symbol]
	trades = append(trades, t)
	// 保留最近 1000 条
	if len(trades) > 1000 {
		trades = trades[len(trades)-1000:]
	}
	s.trades[event.Symbol] = trades

	// 更新 ticker
	ticker := s.tickers[event.Symbol]
	if ticker == nil {
		ticker = &Ticker{
			Symbol:     event.Symbol,
			OpenTimeMs: now,
			HighPrice:  trade.Price,
			LowPrice:   trade.Price,
			OpenPrice:  trade.Price,
		}
		s.tickers[event.Symbol] = ticker
	}

	ticker.LastPrice = trade.Price
	ticker.CloseTimeMs = now
	ticker.Volume += trade.Qty
	ticker.QuoteVolume += trade.Price * trade.Qty / 1e8
	ticker.TradeCount++

	if trade.Price > ticker.HighPrice {
		ticker.HighPrice = trade.Price
	}
	if trade.Price < ticker.LowPrice || ticker.LowPrice == 0 {
		ticker.LowPrice = trade.Price
	}

	ticker.PriceChange = ticker.LastPrice - ticker.OpenPrice
	if ticker.OpenPrice > 0 {
		ticker.PriceChangePercent = formatPercent(float64(ticker.PriceChange) / float64(ticker.OpenPrice) * 100)
	}

	// 更新盘口（从成交中移除数量）
	depth := s.depths[event.Symbol]
	if depth != nil {
		depth.LastUpdateID = event.Seq
		depth.TimestampMs = now
	}

	s.mu.Unlock()

	// 推送成交事件
	s.publish(event.Symbol, "trades", t)
}

func (s *MarketDataService) handleOrderAccepted(event MatchingEvent) {
	var order OrderAcceptedData
	if err := json.Unmarshal(event.Data, &order); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	depth := s.depths[event.Symbol]
	if depth == nil {
		depth = &Depth{
			Symbol: event.Symbol,
			Bids:   []PriceLevel{},
			Asks:   []PriceLevel{},
		}
		s.depths[event.Symbol] = depth
	}

	// 添加到盘口
	level := PriceLevel{Price: order.Price, Qty: order.Qty}
	if order.Side == 1 { // BUY
		depth.Bids = insertLevel(depth.Bids, level, true)
	} else { // SELL
		depth.Asks = insertLevel(depth.Asks, level, false)
	}

	depth.LastUpdateID = event.Seq
	depth.TimestampMs = time.Now().UnixMilli()

	// 推送盘口更新
	s.publishDepth(event.Symbol, depth)
}

func (s *MarketDataService) handleOrderRemoved(event MatchingEvent) {
	// 简化实现：实际需要根据订单信息更新盘口
	s.mu.Lock()
	defer s.mu.Unlock()

	depth := s.depths[event.Symbol]
	if depth != nil {
		depth.LastUpdateID = event.Seq
		depth.TimestampMs = time.Now().UnixMilli()
		s.publishDepth(event.Symbol, depth)
	}
}

func (s *MarketDataService) publish(symbol, dataType string, data interface{}) {
	channel := "market." + symbol + "." + dataType
	event := &Event{
		Channel:     channel,
		Seq:         time.Now().UnixNano(),
		TimestampMs: time.Now().UnixMilli(),
		Data:        data,
	}

	s.subMu.RLock()
	subs := s.subscribers[channel]
	s.subMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// 队列满，丢弃
		}
	}
}

func (s *MarketDataService) publishDepth(symbol string, depth *Depth) {
	channel := "market." + symbol + ".book"
	event := &Event{
		Channel:     channel,
		Seq:         depth.LastUpdateID,
		TimestampMs: depth.TimestampMs,
		Data:        depth,
	}

	s.subMu.RLock()
	subs := s.subscribers[channel]
	s.subMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// insertLevel 插入价格档位并保持排序
func insertLevel(levels []PriceLevel, level PriceLevel, descending bool) []PriceLevel {
	// 查找是否已存在该价格
	for i, l := range levels {
		if l.Price == level.Price {
			if level.Qty == 0 {
				// 删除
				return append(levels[:i], levels[i+1:]...)
			}
			// 更新
			levels[i].Qty = level.Qty
			return levels
		}
	}

	if level.Qty == 0 {
		return levels
	}

	// 插入新档位
	i := 0
	for i < len(levels) {
		if descending {
			if level.Price > levels[i].Price {
				break
			}
		} else {
			if level.Price < levels[i].Price {
				break
			}
		}
		i++
	}

	levels = append(levels, PriceLevel{})
	copy(levels[i+1:], levels[i:])
	levels[i] = level
	return levels
}

func formatPercent(p float64) string {
	if p >= 0 {
		return "+" + formatFloat(p) + "%"
	}
	return formatFloat(p) + "%"
}

func formatFloat(f float64) string {
	return string(rune(int(f*100))) + "." + string(rune(int(f*10000)%100))
}
