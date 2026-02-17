// Package service 行情服务
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/exchange/common/pkg/health"
	"github.com/redis/go-redis/v9"
)

// MarketDataService 行情服务
type MarketDataService struct {
	redis       RedisClient
	eventStream string
	group       string
	consumer    string
	replayCount int

	loop health.LoopMonitor

	// 内存盘口
	depths map[string]*Depth
	mu     sync.RWMutex
	// 盘口订单索引（symbol -> orderID -> order level），用于增量更新深度。
	openOrders map[string]map[int64]*orderLevel

	// 最近成交
	trades map[string][]*Trade

	// 24h ticker
	tickers map[string]*Ticker

	// 订阅者
	subscribers map[string][]chan *Event
	subMu       sync.RWMutex
}

type orderLevel struct {
	OrderID   int64
	Side      int
	Price     int64
	LeavesQty int64
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
	ReplayCount int
}

// NewMarketDataService 创建行情服务
func NewMarketDataService(redisClient RedisClient, cfg *Config) *MarketDataService {
	return &MarketDataService{
		redis:       redisClient,
		eventStream: cfg.EventStream,
		group:       cfg.Group,
		consumer:    cfg.Consumer,
		replayCount: cfg.ReplayCount,
		depths:      make(map[string]*Depth),
		trades:      make(map[string][]*Trade),
		tickers:     make(map[string]*Ticker),
		openOrders:  make(map[string]map[int64]*orderLevel),
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

	if s.replayCount > 0 {
		if err := s.replayRecent(ctx, s.replayCount); err != nil {
			log.Printf("replay events error: %v", err)
		}
	}

	s.loop.Tick()
	go s.consumeEvents(ctx)
	return nil
}

func (s *MarketDataService) ConsumeLoopHealthy(now time.Time, maxAge time.Duration) (bool, time.Duration, string) {
	return s.loop.Healthy(now, maxAge)
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
	result := cloneDepth(depth, limit)
	if result == nil {
		return &Depth{Symbol: symbol, Bids: []PriceLevel{}, Asks: []PriceLevel{}}
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
		return cloneTrades(trades)
	}
	return cloneTrades(trades[len(trades)-limit:])
}

// GetTicker 获取 24h 行情
func (s *MarketDataService) GetTicker(symbol string) *Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ticker, ok := s.tickers[symbol]
	if !ok {
		return &Ticker{Symbol: symbol}
	}
	snapshot := *ticker
	return &snapshot
}

// GetAllTickers 获取所有 ticker
func (s *MarketDataService) GetAllTickers() []*Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Ticker, 0, len(s.tickers))
	for _, t := range s.tickers {
		snapshot := *t
		result = append(result, &snapshot)
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
	defer func() {
		if r := recover(); r != nil {
			s.loop.SetError(fmt.Errorf("panic: %v", r))
			log.Printf("consumeEvents panic: %v\n%s", r, string(debug.Stack()))
		}
	}()

	log.Printf("Consuming events from %s", s.eventStream)

	pendingTicker := time.NewTicker(30 * time.Second)
	defer pendingTicker.Stop()

	if err := s.processPending(ctx); err != nil {
		s.loop.SetError(err)
		log.Printf("Process pending error: %v", err)
	}

	for {
		s.loop.Tick()

		select {
		case <-ctx.Done():
			return
		case <-pendingTicker.C:
			if err := s.processPending(ctx); err != nil {
				s.loop.SetError(err)
				log.Printf("Process pending error: %v", err)
			}
			continue
		default:
		}

		results, err := s.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.group,
			Consumer: s.consumer,
			Streams:  []string{s.eventStream, ">"},
			Count:    100,
			Block:    1000 * time.Millisecond,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			s.loop.SetError(err)
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

func (s *MarketDataService) processPending(ctx context.Context) error {
	pending, err := s.redis.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: s.eventStream,
		Group:  s.group,
		Start:  "-",
		End:    "+",
		Count:  100,
	}).Result()
	if err != nil {
		return err
	}

	var ids []string
	for _, entry := range pending {
		if entry.Idle >= 30*time.Second {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	claimed, err := s.redis.XClaim(ctx, &redis.XClaimArgs{
		Stream:   s.eventStream,
		Group:    s.group,
		Consumer: s.consumer,
		MinIdle:  30 * time.Second,
		Messages: ids,
	}).Result()
	if err != nil {
		return err
	}

	for _, msg := range claimed {
		s.processEvent(ctx, msg)
	}
	return nil
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

type OrderPartiallyFilledData struct {
	OrderID   int64 `json:"OrderID"`
	UserID    int64 `json:"UserID"`
	LeavesQty int64 `json:"LeavesQty"`
}

type OrderFilledData struct {
	OrderID int64 `json:"OrderID"`
	UserID  int64 `json:"UserID"`
}

func (s *MarketDataService) processEvent(ctx context.Context, msg redis.XMessage) {
	data, ok := msg.Values["data"].(string)
	if !ok {
		s.redis.XAck(ctx, s.eventStream, s.group, msg.ID)
		return
	}

	if err := s.processEventData(data); err != nil {
		log.Printf("process event error: %v", err)
	}
	s.redis.XAck(ctx, s.eventStream, s.group, msg.ID)
}

func (s *MarketDataService) processEventData(data string) error {
	var event MatchingEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	switch event.Type {
	case "TRADE_CREATED":
		s.handleTradeCreated(event)
	case "ORDER_ACCEPTED":
		s.handleOrderAccepted(event)
	case "ORDER_PARTIALLY_FILLED":
		s.handleOrderPartiallyFilled(event)
	case "ORDER_CANCELED", "ORDER_FILLED":
		s.handleOrderRemoved(event)
	}

	return nil
}

func (s *MarketDataService) replayRecent(ctx context.Context, count int) error {
	if count <= 0 {
		return nil
	}

	results, err := s.redis.XRevRangeN(ctx, s.eventStream, "+", "-", int64(count)).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	for i := len(results) - 1; i >= 0; i-- {
		data, ok := results[i].Values["data"].(string)
		if !ok {
			continue
		}
		if err := s.processEventData(data); err != nil {
			log.Printf("replay event error: %v", err)
		}
	}
	return nil
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
		depth.Bids = applyLevelDelta(depth.Bids, level.Price, level.Qty, true)
	} else { // SELL
		depth.Asks = applyLevelDelta(depth.Asks, level.Price, level.Qty, false)
	}
	if _, ok := s.openOrders[event.Symbol]; !ok {
		s.openOrders[event.Symbol] = make(map[int64]*orderLevel)
	}
	s.openOrders[event.Symbol][order.OrderID] = &orderLevel{
		OrderID:   order.OrderID,
		Side:      order.Side,
		Price:     order.Price,
		LeavesQty: order.Qty,
	}

	depth.LastUpdateID = event.Seq
	depth.TimestampMs = time.Now().UnixMilli()

	// 推送盘口更新
	s.publishDepth(event.Symbol, depth)
}

func (s *MarketDataService) handleOrderPartiallyFilled(event MatchingEvent) {
	var data OrderPartiallyFilledData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	depth := s.depths[event.Symbol]
	if depth == nil {
		return
	}

	ordersBySymbol, ok := s.openOrders[event.Symbol]
	if !ok {
		return
	}
	entry, ok := ordersBySymbol[data.OrderID]
	if !ok || entry == nil {
		return
	}

	leavesQty := data.LeavesQty
	if leavesQty < 0 {
		leavesQty = 0
	}

	deltaQty := leavesQty - entry.LeavesQty
	if entry.Side == 1 {
		depth.Bids = applyLevelDelta(depth.Bids, entry.Price, deltaQty, true)
	} else {
		depth.Asks = applyLevelDelta(depth.Asks, entry.Price, deltaQty, false)
	}

	if leavesQty == 0 {
		delete(ordersBySymbol, data.OrderID)
		if len(ordersBySymbol) == 0 {
			delete(s.openOrders, event.Symbol)
		}
	} else {
		entry.LeavesQty = leavesQty
	}

	depth.LastUpdateID = event.Seq
	depth.TimestampMs = time.Now().UnixMilli()
	s.publishDepth(event.Symbol, depth)
}

func (s *MarketDataService) handleOrderRemoved(event MatchingEvent) {
	var orderID int64
	switch event.Type {
	case "ORDER_CANCELED":
		var data OrderCanceledData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return
		}
		orderID = data.OrderID
	case "ORDER_FILLED":
		var data OrderFilledData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return
		}
		orderID = data.OrderID
	default:
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	depth := s.depths[event.Symbol]
	if depth == nil {
		return
	}

	if ordersBySymbol, ok := s.openOrders[event.Symbol]; ok {
		if entry, exists := ordersBySymbol[orderID]; exists && entry != nil {
			deltaQty := -entry.LeavesQty
			if entry.Side == 1 {
				depth.Bids = applyLevelDelta(depth.Bids, entry.Price, deltaQty, true)
			} else {
				depth.Asks = applyLevelDelta(depth.Asks, entry.Price, deltaQty, false)
			}
			delete(ordersBySymbol, orderID)
			if len(ordersBySymbol) == 0 {
				delete(s.openOrders, event.Symbol)
			}
		}
	}

	depth.LastUpdateID = event.Seq
	depth.TimestampMs = time.Now().UnixMilli()
	s.publishDepth(event.Symbol, depth)
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
	if depth == nil {
		return
	}
	snapshot := cloneDepth(depth, 0)
	if snapshot == nil {
		return
	}
	channel := "market." + symbol + ".book"
	event := &Event{
		Channel:     channel,
		Seq:         snapshot.LastUpdateID,
		TimestampMs: snapshot.TimestampMs,
		Data:        snapshot,
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

func cloneDepth(depth *Depth, limit int) *Depth {
	if depth == nil {
		return nil
	}
	result := &Depth{
		Symbol:       depth.Symbol,
		LastUpdateID: depth.LastUpdateID,
		TimestampMs:  depth.TimestampMs,
	}
	bids := depth.Bids
	asks := depth.Asks
	if limit > 0 && limit < len(bids) {
		bids = bids[:limit]
	}
	if limit > 0 && limit < len(asks) {
		asks = asks[:limit]
	}
	result.Bids = clonePriceLevels(bids)
	result.Asks = clonePriceLevels(asks)
	return result
}

func clonePriceLevels(levels []PriceLevel) []PriceLevel {
	if len(levels) == 0 {
		return []PriceLevel{}
	}
	result := make([]PriceLevel, len(levels))
	copy(result, levels)
	return result
}

func cloneTrades(trades []*Trade) []*Trade {
	if len(trades) == 0 {
		return []*Trade{}
	}
	result := make([]*Trade, len(trades))
	copy(result, trades)
	return result
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

// applyLevelDelta 按价格档位增量调整数量，保持排序。
// delta > 0: 增量；delta < 0: 减量；调整后 <=0 则移除该档位。
func applyLevelDelta(levels []PriceLevel, price int64, delta int64, descending bool) []PriceLevel {
	if delta == 0 {
		return levels
	}
	for i, current := range levels {
		if current.Price != price {
			continue
		}
		nextQty := current.Qty + delta
		if nextQty <= 0 {
			return append(levels[:i], levels[i+1:]...)
		}
		levels[i].Qty = nextQty
		return levels
	}
	if delta < 0 {
		return levels
	}
	return insertLevel(levels, PriceLevel{Price: price, Qty: delta}, descending)
}

func formatPercent(p float64) string {
	return fmt.Sprintf("%+.2f%%", p)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
