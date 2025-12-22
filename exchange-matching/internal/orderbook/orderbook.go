// Package orderbook 订单簿实现
package orderbook

import (
	"container/list"
	"sync"
	"time"
)

// Side 订单方向
type Side int

const (
	SideBuy  Side = 1
	SideSell Side = 2
)

// Order 订单
type Order struct {
	OrderID       int64
	UserID        int64
	ClientOrderID string
	Symbol        string
	Side          Side
	Price         int64 // 最小单位整数
	OrigQty       int64 // 原始数量
	LeavesQty     int64 // 剩余数量
	TimeInForce   int   // 1=GTC, 2=IOC, 3=FOK, 4=POST_ONLY
	Timestamp     int64 // 纳秒时间戳
	element       *list.Element
}

// PriceLevel 价格档位
type PriceLevel struct {
	Price  int64
	Orders *list.List // *Order
	Total  int64      // 该档位总数量
}

// OrderBook 订单簿
type OrderBook struct {
	Symbol string

	// 买盘：价格降序（高价优先）
	bids map[int64]*PriceLevel
	// 卖盘：价格升序（低价优先）
	asks map[int64]*PriceLevel

	// 订单索引
	orders map[int64]*Order

	// 价格排序缓存
	bidPrices []int64
	askPrices []int64

	mu sync.RWMutex

	// 序列号
	seq int64
}

// NewOrderBook 创建订单簿
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol:    symbol,
		bids:      make(map[int64]*PriceLevel),
		asks:      make(map[int64]*PriceLevel),
		orders:    make(map[int64]*Order),
		bidPrices: make([]int64, 0),
		askPrices: make([]int64, 0),
	}
}

// AddOrder 添加订单到订单簿
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order.Timestamp = time.Now().UnixNano()

	var levels map[int64]*PriceLevel
	var prices *[]int64
	if order.Side == SideBuy {
		levels = ob.bids
		prices = &ob.bidPrices
	} else {
		levels = ob.asks
		prices = &ob.askPrices
	}

	level, exists := levels[order.Price]
	if !exists {
		level = &PriceLevel{
			Price:  order.Price,
			Orders: list.New(),
		}
		levels[order.Price] = level
		*prices = insertPrice(*prices, order.Price, order.Side == SideBuy)
	}

	order.element = level.Orders.PushBack(order)
	level.Total += order.LeavesQty
	ob.orders[order.OrderID] = order
}

// RemoveOrder 从订单簿移除订单
func (ob *OrderBook) RemoveOrder(orderID int64) *Order {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	return ob.removeOrderLocked(orderID)
}

func (ob *OrderBook) removeOrderLocked(orderID int64) *Order {
	order, exists := ob.orders[orderID]
	if !exists {
		return nil
	}

	var levels map[int64]*PriceLevel
	var prices *[]int64
	if order.Side == SideBuy {
		levels = ob.bids
		prices = &ob.bidPrices
	} else {
		levels = ob.asks
		prices = &ob.askPrices
	}

	level := levels[order.Price]
	if level != nil {
		level.Orders.Remove(order.element)
		level.Total -= order.LeavesQty

		if level.Orders.Len() == 0 {
			delete(levels, order.Price)
			*prices = removePrice(*prices, order.Price)
		}
	}

	delete(ob.orders, orderID)
	return order
}

// ReduceOrderQty 减少订单数量（部分成交）
func (ob *OrderBook) ReduceOrderQty(orderID int64, qty int64) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order, exists := ob.orders[orderID]
	if !exists {
		return
	}

	var levels map[int64]*PriceLevel
	if order.Side == SideBuy {
		levels = ob.bids
	} else {
		levels = ob.asks
	}

	level := levels[order.Price]
	if level != nil {
		level.Total -= qty
	}
	order.LeavesQty -= qty

	if order.LeavesQty <= 0 {
		ob.removeOrderLocked(orderID)
	}
}

// GetOrder 获取订单
func (ob *OrderBook) GetOrder(orderID int64) *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.orders[orderID]
}

// BestBid 最优买价
func (ob *OrderBook) BestBid() (int64, int64, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.bidPrices) == 0 {
		return 0, 0, false
	}
	price := ob.bidPrices[0]
	level := ob.bids[price]
	return price, level.Total, true
}

// BestAsk 最优卖价
func (ob *OrderBook) BestAsk() (int64, int64, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.askPrices) == 0 {
		return 0, 0, false
	}
	price := ob.askPrices[0]
	level := ob.asks[price]
	return price, level.Total, true
}

// Depth 获取深度
func (ob *OrderBook) Depth(limit int) (bids, asks []PriceQty) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids = make([]PriceQty, 0, limit)
	asks = make([]PriceQty, 0, limit)

	for i := 0; i < len(ob.bidPrices) && i < limit; i++ {
		price := ob.bidPrices[i]
		level := ob.bids[price]
		bids = append(bids, PriceQty{Price: price, Qty: level.Total})
	}

	for i := 0; i < len(ob.askPrices) && i < limit; i++ {
		price := ob.askPrices[i]
		level := ob.asks[price]
		asks = append(asks, PriceQty{Price: price, Qty: level.Total})
	}

	return
}

// PriceQty 价格数量对
type PriceQty struct {
	Price int64 `json:"price"`
	Qty   int64 `json:"qty"`
}

// MatchResult 撮合结果
type MatchResult struct {
	Trades       []*Trade
	MakerUpdates []*Order // 被动方订单更新
	TakerOrder   *Order   // 主动方订单
	TakerFilled  bool     // 主动方是否完全成交
}

// Trade 成交
type Trade struct {
	TradeID      int64
	Symbol       string
	MakerOrderID int64
	TakerOrderID int64
	MakerUserID  int64
	TakerUserID  int64
	Price        int64
	Qty          int64
	TakerSide    Side
	Timestamp    int64
}

// Match 撮合订单
func (ob *OrderBook) Match(taker *Order) *MatchResult {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	result := &MatchResult{
		Trades:       make([]*Trade, 0),
		MakerUpdates: make([]*Order, 0),
		TakerOrder:   taker,
	}

	var levels map[int64]*PriceLevel
	var prices *[]int64
	var canMatch func(makerPrice, takerPrice int64) bool

	if taker.Side == SideBuy {
		levels = ob.asks
		prices = &ob.askPrices
		canMatch = func(makerPrice, takerPrice int64) bool {
			return takerPrice == 0 || makerPrice <= takerPrice // 市价单 takerPrice=0
		}
	} else {
		levels = ob.bids
		prices = &ob.bidPrices
		canMatch = func(makerPrice, takerPrice int64) bool {
			return takerPrice == 0 || makerPrice >= takerPrice
		}
	}

	now := time.Now().UnixNano()

	for taker.LeavesQty > 0 && len(*prices) > 0 {
		bestPrice := (*prices)[0]

		if !canMatch(bestPrice, taker.Price) {
			break
		}

		level := levels[bestPrice]
		matchedInLevel := false // 追踪本档位是否有匹配
		for e := level.Orders.Front(); e != nil && taker.LeavesQty > 0; {
			maker := e.Value.(*Order)
			next := e.Next()

			// 自成交检查
			if maker.UserID == taker.UserID {
				e = next
				continue
			}

			matchedInLevel = true

			// 计算成交数量
			matchQty := min(taker.LeavesQty, maker.LeavesQty)

			// 创建成交
			trade := &Trade{
				TradeID:      ob.nextSeq(),
				Symbol:       ob.Symbol,
				MakerOrderID: maker.OrderID,
				TakerOrderID: taker.OrderID,
				MakerUserID:  maker.UserID,
				TakerUserID:  taker.UserID,
				Price:        maker.Price, // 成交价为 maker 价格
				Qty:          matchQty,
				TakerSide:    taker.Side,
				Timestamp:    now,
			}
			result.Trades = append(result.Trades, trade)

			// 更新数量
			taker.LeavesQty -= matchQty
			maker.LeavesQty -= matchQty
			level.Total -= matchQty

			result.MakerUpdates = append(result.MakerUpdates, maker)

			// 移除完全成交的 maker
			if maker.LeavesQty <= 0 {
				level.Orders.Remove(e)
				delete(ob.orders, maker.OrderID)
			}

			e = next
		}

		// 如果本档位没有任何匹配（全是自成交），跳出循环
		if !matchedInLevel {
			break
		}

		// 移除空档位
		if level.Orders.Len() == 0 {
			delete(levels, bestPrice)
			*prices = (*prices)[1:]
		}
	}

	result.TakerFilled = taker.LeavesQty <= 0
	return result
}

func (ob *OrderBook) nextSeq() int64 {
	ob.seq++
	return ob.seq
}

// insertPrice 插入价格并保持排序
func insertPrice(prices []int64, price int64, descending bool) []int64 {
	i := 0
	for i < len(prices) {
		if descending {
			if price > prices[i] {
				break
			}
		} else {
			if price < prices[i] {
				break
			}
		}
		i++
	}

	prices = append(prices, 0)
	copy(prices[i+1:], prices[i:])
	prices[i] = price
	return prices
}

// removePrice 移除价格
func removePrice(prices []int64, price int64) []int64 {
	for i, p := range prices {
		if p == price {
			return append(prices[:i], prices[i+1:]...)
		}
	}
	return prices
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
