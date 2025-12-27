// Package engine 撮合引擎
package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/exchange/matching/internal/orderbook"
)

// Command 命令类型
type CommandType int

const (
	CmdNewOrder CommandType = iota + 1
	CmdCancelOrder
)

// Command 撮合命令
type Command struct {
	Type          CommandType
	OrderID       int64
	ClientOrderID string
	UserID        int64
	Symbol        string
	Side          orderbook.Side
	OrderType     int // 1=LIMIT, 2=MARKET
	TimeInForce   int // 1=GTC, 2=IOC, 3=FOK, 4=POST_ONLY
	Price         int64
	Qty           int64
}

// Event 撮合事件
type Event struct {
	Type      EventType
	Symbol    string
	Seq       int64
	Timestamp int64
	Data      interface{}
}

// EventType 事件类型
type EventType int

const (
	EventOrderAccepted EventType = iota + 1
	EventOrderRejected
	EventOrderCanceled
	EventTradeCreated
	EventOrderFilled
	EventOrderPartiallyFilled
)

// OrderAcceptedData 订单接受事件数据
type OrderAcceptedData struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	Side          orderbook.Side
	Price         int64
	Qty           int64
}

// OrderRejectedData 订单拒绝事件数据
type OrderRejectedData struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	Reason        string
}

// OrderCanceledData 订单取消事件数据
type OrderCanceledData struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	LeavesQty     int64
	Reason        string
}

// TradeCreatedData 成交事件数据
type TradeCreatedData struct {
	TradeID      int64
	MakerOrderID int64
	TakerOrderID int64
	MakerUserID  int64
	TakerUserID  int64
	Price        int64
	Qty          int64
	TakerSide    orderbook.Side
}

// OrderFilledData 订单完全成交事件数据
type OrderFilledData struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	ExecutedQty   int64
}

// OrderPartiallyFilledData 订单部分成交事件数据
type OrderPartiallyFilledData struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	ExecutedQty   int64
	LeavesQty     int64
}

// Engine 撮合引擎
type Engine struct {
	symbol string
	book   *orderbook.OrderBook

	cmdCh   chan *Command
	eventCh chan *Event

	seq int64
	mu  sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewEngine 创建撮合引擎
func NewEngine(symbol string, cmdBufferSize, eventBufferSize int) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		symbol:  symbol,
		book:    orderbook.NewOrderBook(symbol),
		cmdCh:   make(chan *Command, cmdBufferSize),
		eventCh: make(chan *Event, eventBufferSize),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动引擎
func (e *Engine) Start() {
	go e.run()
}

// Stop 停止引擎
func (e *Engine) Stop() {
	e.cancel()
}

// Submit 提交命令
func (e *Engine) Submit(cmd *Command) error {
	// 先检查是否已停止
	select {
	case <-e.ctx.Done():
		return fmt.Errorf("engine stopped")
	default:
	}

	// 尝试发送命令
	select {
	case e.cmdCh <- cmd:
		return nil
	case <-e.ctx.Done():
		return fmt.Errorf("engine stopped")
	default:
		return fmt.Errorf("command queue full")
	}
}

// Events 获取事件通道
func (e *Engine) Events() <-chan *Event {
	return e.eventCh
}

func (e *Engine) Done() <-chan struct{} {
	return e.ctx.Done()
}

// Depth 获取深度
func (e *Engine) Depth(limit int) (bids, asks []orderbook.PriceQty) {
	return e.book.Depth(limit)
}

func (e *Engine) run() {
	for {
		select {
		case cmd := <-e.cmdCh:
			e.processCommand(cmd)
		case <-e.ctx.Done():
			return
		}
	}
}

func (e *Engine) processCommand(cmd *Command) {
	switch cmd.Type {
	case CmdNewOrder:
		e.processNewOrder(cmd)
	case CmdCancelOrder:
		e.processCancelOrder(cmd)
	}
}

func (e *Engine) processNewOrder(cmd *Command) {
	now := time.Now().UnixNano()

	order := &orderbook.Order{
		OrderID:       cmd.OrderID,
		UserID:        cmd.UserID,
		ClientOrderID: cmd.ClientOrderID,
		Symbol:        cmd.Symbol,
		Side:          cmd.Side,
		Price:         cmd.Price,
		OrigQty:       cmd.Qty,
		LeavesQty:     cmd.Qty,
		TimeInForce:   cmd.TimeInForce,
		Timestamp:     now,
	}

	// 市价单价格设为 0
	if cmd.OrderType == 2 {
		order.Price = 0
	}

	// POST_ONLY 检查
	if cmd.TimeInForce == 4 {
		if e.wouldMatch(order) {
			e.emit(EventOrderRejected, &OrderRejectedData{
				OrderID:       cmd.OrderID,
				ClientOrderID: cmd.ClientOrderID,
				UserID:        cmd.UserID,
				Reason:        "POST_ONLY_REJECTED",
			})
			return
		}
	}

	// 撮合
	result := e.book.Match(order)

	// 发送成交事件
	for _, trade := range result.Trades {
		e.emit(EventTradeCreated, &TradeCreatedData{
			TradeID:      trade.TradeID,
			MakerOrderID: trade.MakerOrderID,
			TakerOrderID: trade.TakerOrderID,
			MakerUserID:  trade.MakerUserID,
			TakerUserID:  trade.TakerUserID,
			Price:        trade.Price,
			Qty:          trade.Qty,
			TakerSide:    trade.TakerSide,
		})
	}

	// 发送 maker 更新事件
	for _, maker := range result.MakerUpdates {
		if maker.LeavesQty <= 0 {
			e.emit(EventOrderFilled, &OrderFilledData{
				OrderID:       maker.OrderID,
				ClientOrderID: maker.ClientOrderID,
				UserID:        maker.UserID,
				ExecutedQty:   maker.OrigQty,
			})
		} else {
			e.emit(EventOrderPartiallyFilled, &OrderPartiallyFilledData{
				OrderID:       maker.OrderID,
				ClientOrderID: maker.ClientOrderID,
				UserID:        maker.UserID,
				ExecutedQty:   maker.OrigQty - maker.LeavesQty,
				LeavesQty:     maker.LeavesQty,
			})
		}
	}

	// 处理 taker
	executedQty := order.OrigQty - order.LeavesQty

	if result.TakerFilled {
		// 完全成交
		e.emit(EventOrderFilled, &OrderFilledData{
			OrderID:       order.OrderID,
			ClientOrderID: order.ClientOrderID,
			UserID:        order.UserID,
			ExecutedQty:   executedQty,
		})
	} else if executedQty > 0 {
		// 部分成交
		e.emit(EventOrderPartiallyFilled, &OrderPartiallyFilledData{
			OrderID:       order.OrderID,
			ClientOrderID: order.ClientOrderID,
			UserID:        order.UserID,
			ExecutedQty:   executedQty,
			LeavesQty:     order.LeavesQty,
		})

		// IOC: 取消剩余
		if cmd.TimeInForce == 2 {
			e.emit(EventOrderCanceled, &OrderCanceledData{
				OrderID:       order.OrderID,
				ClientOrderID: order.ClientOrderID,
				UserID:        order.UserID,
				LeavesQty:     order.LeavesQty,
				Reason:        "IOC_EXPIRED",
			})
		} else if cmd.OrderType == 1 { // 限价单挂单
			e.book.AddOrder(order)
			e.emit(EventOrderAccepted, &OrderAcceptedData{
				OrderID:       order.OrderID,
				ClientOrderID: order.ClientOrderID,
				UserID:        order.UserID,
				Side:          order.Side,
				Price:         order.Price,
				Qty:           order.LeavesQty,
			})
		}
	} else {
		// 无成交
		if cmd.TimeInForce == 2 || cmd.TimeInForce == 3 { // IOC/FOK
			e.emit(EventOrderRejected, &OrderRejectedData{
				OrderID:       cmd.OrderID,
				ClientOrderID: cmd.ClientOrderID,
				UserID:        cmd.UserID,
				Reason:        "NO_LIQUIDITY",
			})
		} else if cmd.OrderType == 1 { // 限价单挂单
			e.book.AddOrder(order)
			e.emit(EventOrderAccepted, &OrderAcceptedData{
				OrderID:       order.OrderID,
				ClientOrderID: order.ClientOrderID,
				UserID:        order.UserID,
				Side:          order.Side,
				Price:         order.Price,
				Qty:           order.LeavesQty,
			})
		} else { // 市价单无流动性
			e.emit(EventOrderRejected, &OrderRejectedData{
				OrderID:       cmd.OrderID,
				ClientOrderID: cmd.ClientOrderID,
				UserID:        cmd.UserID,
				Reason:        "NO_LIQUIDITY",
			})
		}
	}
}

func (e *Engine) processCancelOrder(cmd *Command) {
	order := e.book.RemoveOrder(cmd.OrderID)
	if order == nil {
		e.emit(EventOrderRejected, &OrderRejectedData{
			OrderID:       cmd.OrderID,
			ClientOrderID: cmd.ClientOrderID,
			UserID:        cmd.UserID,
			Reason:        "ORDER_NOT_FOUND",
		})
		return
	}

	e.emit(EventOrderCanceled, &OrderCanceledData{
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		UserID:        order.UserID,
		LeavesQty:     order.LeavesQty,
		Reason:        "USER_CANCELED",
	})
}

func (e *Engine) wouldMatch(order *orderbook.Order) bool {
	if order.Side == orderbook.SideBuy {
		bestAsk, _, ok := e.book.BestAsk()
		return ok && order.Price >= bestAsk
	}
	bestBid, _, ok := e.book.BestBid()
	return ok && order.Price <= bestBid
}

func (e *Engine) emit(eventType EventType, data interface{}) {
	e.mu.Lock()
	e.seq++
	seq := e.seq
	e.mu.Unlock()

	event := &Event{
		Type:      eventType,
		Symbol:    e.symbol,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Data:      data,
	}

	select {
	case e.eventCh <- event:
	case <-e.ctx.Done():
	}
}
