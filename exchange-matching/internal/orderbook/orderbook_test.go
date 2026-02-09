package orderbook

import (
	"testing"
)

func TestSideConstants(t *testing.T) {
	if SideBuy != 1 {
		t.Fatalf("expected SideBuy=1, got %d", SideBuy)
	}
	if SideSell != 2 {
		t.Fatalf("expected SideSell=2, got %d", SideSell)
	}
}

func TestNewOrderBook(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")
	if ob == nil {
		t.Fatal("expected non-nil orderbook")
	}
	if ob.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", ob.Symbol)
	}
}

func TestOrderStruct(t *testing.T) {
	order := &Order{
		OrderID:       1,
		UserID:        100,
		ClientOrderID: "client-1",
		Symbol:        "BTCUSDT",
		Side:          SideBuy,
		Price:         50000,
		OrigQty:       100,
		LeavesQty:     100,
		TimeInForce:   1,
		Timestamp:     1000000,
	}

	if order.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", order.OrderID)
	}
	if order.Side != SideBuy {
		t.Fatalf("expected Side=SideBuy, got %d", order.Side)
	}
	if order.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", order.Price)
	}
}

func TestInsertPrice_MiddleInsert(t *testing.T) {
	// 升序插入
	prices := []int64{}
	prices = insertPrice(prices, 100, false)
	prices = insertPrice(prices, 50, false)
	prices = insertPrice(prices, 150, false)

	expected := []int64{50, 100, 150}
	for i, p := range expected {
		if prices[i] != p {
			t.Errorf("asc[%d]: expected %d, got %d", i, p, prices[i])
		}
	}

	// 降序插入
	prices = []int64{}
	prices = insertPrice(prices, 100, true)
	prices = insertPrice(prices, 50, true)
	prices = insertPrice(prices, 150, true)

	expected = []int64{150, 100, 50}
	for i, p := range expected {
		if prices[i] != p {
			t.Errorf("desc[%d]: expected %d, got %d", i, p, prices[i])
		}
	}
}

func TestRemovePrice_RemoveMiddle(t *testing.T) {
	prices := []int64{50, 100, 150, 200}

	// 移除中间
	result := removePrice(prices, 100)
	if len(result) != 3 {
		t.Errorf("expected len 3, got %d", len(result))
	}

	// 移除不存在
	result = removePrice([]int64{50, 150}, 100)
	if len(result) != 2 {
		t.Error("should not change when price not found")
	}

	// 空切片
	result = removePrice([]int64{}, 100)
	if len(result) != 0 {
		t.Error("empty slice should remain empty")
	}
}

func TestPriceLevelStruct(t *testing.T) {
	level := &PriceLevel{
		Price: 50000,
		Total: 1000,
	}

	if level.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", level.Price)
	}
	if level.Total != 1000 {
		t.Fatalf("expected Total=1000, got %d", level.Total)
	}
}

func TestPriceQtyStruct(t *testing.T) {
	pq := PriceQty{Price: 50000, Qty: 100}
	if pq.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", pq.Price)
	}
	if pq.Qty != 100 {
		t.Fatalf("expected Qty=100, got %d", pq.Qty)
	}
}

func TestTradeStruct(t *testing.T) {
	trade := &Trade{
		TradeID:      1,
		Symbol:       "BTCUSDT",
		MakerOrderID: 100,
		TakerOrderID: 200,
		MakerUserID:  10,
		TakerUserID:  20,
		Price:        50000,
		Qty:          50,
		TakerSide:    SideBuy,
		Timestamp:    1000000,
	}

	if trade.TradeID != 1 {
		t.Fatalf("expected TradeID=1, got %d", trade.TradeID)
	}
	if trade.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", trade.Price)
	}
	if trade.TakerSide != SideBuy {
		t.Fatalf("expected TakerSide=SideBuy, got %d", trade.TakerSide)
	}
}

func TestMatchResultStruct(t *testing.T) {
	result := &MatchResult{
		Trades:       make([]*Trade, 0),
		MakerUpdates: make([]*Order, 0),
		TakerFilled:  true,
	}

	if !result.TakerFilled {
		t.Fatal("expected TakerFilled=true")
	}
	if len(result.Trades) != 0 {
		t.Fatalf("expected empty Trades, got %d", len(result.Trades))
	}
}

func TestAddOrder(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	order := &Order{
		OrderID:   1,
		UserID:    100,
		Symbol:    "BTCUSDT",
		Side:      SideBuy,
		Price:     50000,
		OrigQty:   100,
		LeavesQty: 100,
	}

	ob.AddOrder(order)

	retrieved := ob.GetOrder(1)
	if retrieved == nil {
		t.Fatal("expected to retrieve order")
	}
	if retrieved.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", retrieved.OrderID)
	}
}

func TestRemoveOrder(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	order := &Order{
		OrderID:   1,
		UserID:    100,
		Symbol:    "BTCUSDT",
		Side:      SideBuy,
		Price:     50000,
		OrigQty:   100,
		LeavesQty: 100,
	}

	ob.AddOrder(order)
	removed := ob.RemoveOrder(1)

	if removed == nil {
		t.Fatal("expected to remove order")
	}
	if removed.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", removed.OrderID)
	}

	// Should not find after removal
	retrieved := ob.GetOrder(1)
	if retrieved != nil {
		t.Fatal("expected nil after removal")
	}
}

func TestRemoveNonExistentOrder(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")
	removed := ob.RemoveOrder(999)
	if removed != nil {
		t.Fatal("expected nil for non-existent order")
	}
}

func TestBestBidEmpty(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")
	_, _, ok := ob.BestBid()
	if ok {
		t.Fatal("expected no best bid for empty orderbook")
	}
}

func TestBestAskEmpty(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")
	_, _, ok := ob.BestAsk()
	if ok {
		t.Fatal("expected no best ask for empty orderbook")
	}
}

func TestBestBid(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	ob.AddOrder(&Order{OrderID: 1, UserID: 100, Side: SideBuy, Price: 50000, LeavesQty: 100})
	ob.AddOrder(&Order{OrderID: 2, UserID: 100, Side: SideBuy, Price: 51000, LeavesQty: 200})

	price, qty, ok := ob.BestBid()
	if !ok {
		t.Fatal("expected best bid")
	}
	if price != 51000 {
		t.Fatalf("expected best bid price=51000, got %d", price)
	}
	if qty != 200 {
		t.Fatalf("expected best bid qty=200, got %d", qty)
	}
}

func TestBestAsk(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	ob.AddOrder(&Order{OrderID: 1, UserID: 100, Side: SideSell, Price: 52000, LeavesQty: 100})
	ob.AddOrder(&Order{OrderID: 2, UserID: 100, Side: SideSell, Price: 51000, LeavesQty: 200})

	price, qty, ok := ob.BestAsk()
	if !ok {
		t.Fatal("expected best ask")
	}
	if price != 51000 {
		t.Fatalf("expected best ask price=51000, got %d", price)
	}
	if qty != 200 {
		t.Fatalf("expected best ask qty=200, got %d", qty)
	}
}

func TestDepth(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	ob.AddOrder(&Order{OrderID: 1, UserID: 100, Side: SideBuy, Price: 50000, LeavesQty: 100})
	ob.AddOrder(&Order{OrderID: 2, UserID: 100, Side: SideBuy, Price: 49000, LeavesQty: 200})
	ob.AddOrder(&Order{OrderID: 3, UserID: 100, Side: SideSell, Price: 51000, LeavesQty: 150})

	bids, asks := ob.Depth(10)

	if len(bids) != 2 {
		t.Fatalf("expected 2 bid levels, got %d", len(bids))
	}
	if len(asks) != 1 {
		t.Fatalf("expected 1 ask level, got %d", len(asks))
	}
}

func TestDepthLimit(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	for i := 0; i < 10; i++ {
		ob.AddOrder(&Order{
			OrderID:   int64(i + 1),
			UserID:    100,
			Side:      SideBuy,
			Price:     int64(50000 - i*100),
			LeavesQty: 100,
		})
	}

	bids, _ := ob.Depth(5)
	if len(bids) != 5 {
		t.Fatalf("expected 5 bid levels with limit, got %d", len(bids))
	}
}

func TestReduceOrderQty(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	ob.AddOrder(&Order{OrderID: 1, UserID: 100, Side: SideBuy, Price: 50000, OrigQty: 100, LeavesQty: 100})

	ob.ReduceOrderQty(1, 30)

	order := ob.GetOrder(1)
	if order == nil {
		t.Fatal("expected order to exist")
	}
	if order.LeavesQty != 70 {
		t.Fatalf("expected LeavesQty=70, got %d", order.LeavesQty)
	}
}

func TestReduceOrderQtyToZero(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	ob.AddOrder(&Order{OrderID: 1, UserID: 100, Side: SideBuy, Price: 50000, OrigQty: 100, LeavesQty: 100})

	ob.ReduceOrderQty(1, 100)

	order := ob.GetOrder(1)
	if order != nil {
		t.Fatal("expected order to be removed when qty reaches zero")
	}
}

func TestMatchBuyOrder(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	// Add sell order (maker)
	ob.AddOrder(&Order{
		OrderID:   1,
		UserID:    100,
		Side:      SideSell,
		Price:     50000,
		OrigQty:   100,
		LeavesQty: 100,
	})

	// Match with buy order (taker)
	taker := &Order{
		OrderID:   2,
		UserID:    200,
		Side:      SideBuy,
		Price:     50000,
		OrigQty:   50,
		LeavesQty: 50,
	}

	result := ob.Match(taker)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.Trades[0].Qty != 50 {
		t.Fatalf("expected trade qty=50, got %d", result.Trades[0].Qty)
	}
	if !result.TakerFilled {
		t.Fatal("expected taker to be filled")
	}
}

func TestMatchSellOrder(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	// Add buy order (maker)
	ob.AddOrder(&Order{
		OrderID:   1,
		UserID:    100,
		Side:      SideBuy,
		Price:     50000,
		OrigQty:   100,
		LeavesQty: 100,
	})

	// Match with sell order (taker)
	taker := &Order{
		OrderID:   2,
		UserID:    200,
		Side:      SideSell,
		Price:     50000,
		OrigQty:   50,
		LeavesQty: 50,
	}

	result := ob.Match(taker)

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}
	if result.TakerFilled != true {
		t.Fatal("expected taker to be filled")
	}
}

func TestMatchNoMatch(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	// Add sell order at high price
	ob.AddOrder(&Order{
		OrderID:   1,
		UserID:    100,
		Side:      SideSell,
		Price:     55000,
		OrigQty:   100,
		LeavesQty: 100,
	})

	// Try to match with buy order at lower price
	taker := &Order{
		OrderID:   2,
		UserID:    200,
		Side:      SideBuy,
		Price:     50000,
		OrigQty:   50,
		LeavesQty: 50,
	}

	result := ob.Match(taker)

	if len(result.Trades) != 0 {
		t.Fatalf("expected no trades, got %d", len(result.Trades))
	}
	if result.TakerFilled {
		t.Fatal("expected taker not to be filled")
	}
}

func TestMatchSelfTradePrevention(t *testing.T) {
	ob := NewOrderBook("BTCUSDT")

	// Add sell order from user 100
	ob.AddOrder(&Order{
		OrderID:   1,
		UserID:    100,
		Side:      SideSell,
		Price:     50000,
		OrigQty:   100,
		LeavesQty: 100,
	})

	// Try to match with buy order from same user
	taker := &Order{
		OrderID:   2,
		UserID:    100, // Same user
		Side:      SideBuy,
		Price:     50000,
		OrigQty:   50,
		LeavesQty: 50,
	}

	result := ob.Match(taker)

	if len(result.Trades) != 0 {
		t.Fatalf("expected no trades due to self-trade prevention, got %d", len(result.Trades))
	}
}

func TestInsertPrice_BidAskMiddle(t *testing.T) {
	// Test descending (bids)
	prices := []int64{50000, 49000, 48000}
	prices = insertPrice(prices, 49500, true)
	if prices[1] != 49500 {
		t.Fatalf("expected 49500 at index 1, got %d", prices[1])
	}

	// Test ascending (asks)
	prices = []int64{50000, 51000, 52000}
	prices = insertPrice(prices, 50500, false)
	if prices[1] != 50500 {
		t.Fatalf("expected 50500 at index 1, got %d", prices[1])
	}
}

func TestRemovePrice_AscendingInput(t *testing.T) {
	prices := []int64{48000, 49000, 50000}
	prices = removePrice(prices, 49000)
	if len(prices) != 2 {
		t.Fatalf("expected 2 prices, got %d", len(prices))
	}
	if prices[1] != 50000 {
		t.Fatalf("expected 50000 at index 1, got %d", prices[1])
	}
}

func TestMin(t *testing.T) {
	if min(10, 20) != 10 {
		t.Fatal("expected min(10, 20) = 10")
	}
	if min(20, 10) != 10 {
		t.Fatal("expected min(20, 10) = 10")
	}
	if min(10, 10) != 10 {
		t.Fatal("expected min(10, 10) = 10")
	}
}
