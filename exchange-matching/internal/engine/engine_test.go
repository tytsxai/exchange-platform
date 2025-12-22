package engine

import (
	"testing"
	"time"

	"github.com/exchange/matching/internal/orderbook"
)

func TestCommandTypeConstants(t *testing.T) {
	if CmdNewOrder != 1 {
		t.Fatalf("expected CmdNewOrder=1, got %d", CmdNewOrder)
	}
	if CmdCancelOrder != 2 {
		t.Fatalf("expected CmdCancelOrder=2, got %d", CmdCancelOrder)
	}
}

func TestEventTypeConstants(t *testing.T) {
	if EventOrderAccepted != 1 {
		t.Fatalf("expected EventOrderAccepted=1, got %d", EventOrderAccepted)
	}
	if EventOrderRejected != 2 {
		t.Fatalf("expected EventOrderRejected=2, got %d", EventOrderRejected)
	}
	if EventOrderCanceled != 3 {
		t.Fatalf("expected EventOrderCanceled=3, got %d", EventOrderCanceled)
	}
	if EventTradeCreated != 4 {
		t.Fatalf("expected EventTradeCreated=4, got %d", EventTradeCreated)
	}
	if EventOrderFilled != 5 {
		t.Fatalf("expected EventOrderFilled=5, got %d", EventOrderFilled)
	}
	if EventOrderPartiallyFilled != 6 {
		t.Fatalf("expected EventOrderPartiallyFilled=6, got %d", EventOrderPartiallyFilled)
	}
}

func TestCommandStruct(t *testing.T) {
	cmd := &Command{
		Type:          CmdNewOrder,
		OrderID:       1,
		ClientOrderID: "client-1",
		UserID:        100,
		Symbol:        "BTCUSDT",
		Side:          orderbook.SideBuy,
		OrderType:     1,
		TimeInForce:   1,
		Price:         50000,
		Qty:           100,
	}

	if cmd.Type != CmdNewOrder {
		t.Fatalf("expected Type=CmdNewOrder, got %d", cmd.Type)
	}
	if cmd.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", cmd.OrderID)
	}
	if cmd.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", cmd.Symbol)
	}
}

func TestEventStruct(t *testing.T) {
	event := &Event{
		Type:      EventOrderAccepted,
		Symbol:    "BTCUSDT",
		Seq:       1,
		Timestamp: 1000000,
		Data:      nil,
	}

	if event.Type != EventOrderAccepted {
		t.Fatalf("expected Type=EventOrderAccepted, got %d", event.Type)
	}
	if event.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", event.Symbol)
	}
}

func TestOrderAcceptedDataStruct(t *testing.T) {
	data := &OrderAcceptedData{
		OrderID:       1,
		ClientOrderID: "client-1",
		UserID:        100,
		Side:          orderbook.SideBuy,
		Price:         50000,
		Qty:           100,
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.Side != orderbook.SideBuy {
		t.Fatalf("expected Side=SideBuy, got %d", data.Side)
	}
}

func TestOrderRejectedDataStruct(t *testing.T) {
	data := &OrderRejectedData{
		OrderID:       1,
		ClientOrderID: "client-1",
		UserID:        100,
		Reason:        "POST_ONLY_REJECTED",
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.Reason != "POST_ONLY_REJECTED" {
		t.Fatalf("expected Reason=POST_ONLY_REJECTED, got %s", data.Reason)
	}
}

func TestOrderCanceledDataStruct(t *testing.T) {
	data := &OrderCanceledData{
		OrderID:       1,
		ClientOrderID: "client-1",
		UserID:        100,
		LeavesQty:     50,
		Reason:        "USER_CANCELED",
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.LeavesQty != 50 {
		t.Fatalf("expected LeavesQty=50, got %d", data.LeavesQty)
	}
	if data.Reason != "USER_CANCELED" {
		t.Fatalf("expected Reason=USER_CANCELED, got %s", data.Reason)
	}
}

func TestTradeCreatedDataStruct(t *testing.T) {
	data := &TradeCreatedData{
		TradeID:      1,
		MakerOrderID: 100,
		TakerOrderID: 200,
		MakerUserID:  10,
		TakerUserID:  20,
		Price:        50000,
		Qty:          50,
		TakerSide:    orderbook.SideBuy,
	}

	if data.TradeID != 1 {
		t.Fatalf("expected TradeID=1, got %d", data.TradeID)
	}
	if data.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", data.Price)
	}
	if data.TakerSide != orderbook.SideBuy {
		t.Fatalf("expected TakerSide=SideBuy, got %d", data.TakerSide)
	}
}

func TestOrderFilledDataStruct(t *testing.T) {
	data := &OrderFilledData{
		OrderID:       1,
		ClientOrderID: "client-1",
		UserID:        100,
		ExecutedQty:   100,
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.ExecutedQty != 100 {
		t.Fatalf("expected ExecutedQty=100, got %d", data.ExecutedQty)
	}
}

func TestOrderPartiallyFilledDataStruct(t *testing.T) {
	data := &OrderPartiallyFilledData{
		OrderID:       1,
		ClientOrderID: "client-1",
		UserID:        100,
		ExecutedQty:   50,
		LeavesQty:     50,
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.ExecutedQty != 50 {
		t.Fatalf("expected ExecutedQty=50, got %d", data.ExecutedQty)
	}
	if data.LeavesQty != 50 {
		t.Fatalf("expected LeavesQty=50, got %d", data.LeavesQty)
	}
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine("BTCUSDT", 100, 100)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.symbol != "BTCUSDT" {
		t.Fatalf("expected symbol=BTCUSDT, got %s", engine.symbol)
	}
}

func TestEngineStartStop(t *testing.T) {
	engine := NewEngine("BTCUSDT", 100, 100)
	engine.Start()
	engine.Stop()
	// Should not panic
}

func TestEngineEvents(t *testing.T) {
	engine := NewEngine("BTCUSDT", 100, 100)
	ch := engine.Events()
	if ch == nil {
		t.Fatal("expected non-nil events channel")
	}
}

func TestEngineDepth(t *testing.T) {
	engine := NewEngine("BTCUSDT", 100, 100)
	bids, asks := engine.Depth(10)
	if bids == nil {
		t.Fatal("expected non-nil bids")
	}
	if asks == nil {
		t.Fatal("expected non-nil asks")
	}
}

func TestEngineSubmitAfterStop(t *testing.T) {
	engine := NewEngine("BTCUSDT", 100, 100)
	engine.Stop()

	cmd := &Command{
		Type:    CmdNewOrder,
		OrderID: 1,
	}

	err := engine.Submit(cmd)
	if err == nil {
		t.Fatal("expected error when submitting to stopped engine")
	}
}

func TestEngineSubmitQueueFull(t *testing.T) {
	engine := NewEngine("BTCUSDT", 1, 100)

	// Fill the queue
	cmd1 := &Command{Type: CmdNewOrder, OrderID: 1}
	engine.Submit(cmd1)

	// This should fail due to full queue
	cmd2 := &Command{Type: CmdNewOrder, OrderID: 2}
	err := engine.Submit(cmd2)
	if err == nil {
		t.Fatal("expected error when queue is full")
	}
}

func TestTimeInForceValues(t *testing.T) {
	// GTC = 1, IOC = 2, FOK = 3, POST_ONLY = 4
	cmd := &Command{TimeInForce: 1}
	if cmd.TimeInForce != 1 {
		t.Fatalf("expected TimeInForce=1 (GTC), got %d", cmd.TimeInForce)
	}

	cmd.TimeInForce = 2
	if cmd.TimeInForce != 2 {
		t.Fatalf("expected TimeInForce=2 (IOC), got %d", cmd.TimeInForce)
	}

	cmd.TimeInForce = 3
	if cmd.TimeInForce != 3 {
		t.Fatalf("expected TimeInForce=3 (FOK), got %d", cmd.TimeInForce)
	}

	cmd.TimeInForce = 4
	if cmd.TimeInForce != 4 {
		t.Fatalf("expected TimeInForce=4 (POST_ONLY), got %d", cmd.TimeInForce)
	}
}

func TestOrderTypeValues(t *testing.T) {
	// LIMIT = 1, MARKET = 2
	cmd := &Command{OrderType: 1}
	if cmd.OrderType != 1 {
		t.Fatalf("expected OrderType=1 (LIMIT), got %d", cmd.OrderType)
	}

	cmd.OrderType = 2
	if cmd.OrderType != 2 {
		t.Fatalf("expected OrderType=2 (MARKET), got %d", cmd.OrderType)
	}
}

func newTestEngine() *Engine {
	engine := NewEngine("BTCUSDT", 10000, 10000)
	engine.Start()
	return engine
}

func submitOrFail(t *testing.T, engine *Engine, cmd *Command) {
	t.Helper()
	if err := engine.Submit(cmd); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
}

func collectUntil(t *testing.T, engine *Engine, timeout time.Duration, done func([]*Event) bool) []*Event {
	t.Helper()
	events := make([]*Event, 0)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if done(events) {
			return events
		}
		select {
		case ev := <-engine.Events():
			events = append(events, ev)
		case <-timer.C:
			t.Fatalf("timeout collecting events (got %d)", len(events))
		}
	}
}

func findEvent(events []*Event, eventType EventType) *Event {
	for _, ev := range events {
		if ev.Type == eventType {
			return ev
		}
	}
	return nil
}

func TestLimitOrderMatchFullFill(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         100,
	})

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      20,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         100,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return len(ev) >= 4
	})

	if events[0].Type != EventOrderAccepted {
		t.Fatalf("expected first event accepted, got %v", events[0].Type)
	}
	if events[1].Type != EventTradeCreated {
		t.Fatalf("expected trade event, got %v", events[1].Type)
	}
	trade := events[1].Data.(*TradeCreatedData)
	if trade.Price != 100 || trade.Qty != 100 {
		t.Fatalf("unexpected trade data price=%d qty=%d", trade.Price, trade.Qty)
	}
	if events[2].Type != EventOrderFilled || events[3].Type != EventOrderFilled {
		t.Fatalf("expected filled events, got %v/%v", events[2].Type, events[3].Type)
	}
}

func TestLimitOrderMatchPartialFill(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         50,
	})

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      20,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         100,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return len(ev) >= 5
	})

	if events[1].Type != EventTradeCreated {
		t.Fatalf("expected trade event, got %v", events[1].Type)
	}
	if events[2].Type != EventOrderFilled {
		t.Fatalf("expected maker filled, got %v", events[2].Type)
	}
	if events[3].Type != EventOrderPartiallyFilled {
		t.Fatalf("expected taker partial, got %v", events[3].Type)
	}
	partial := events[3].Data.(*OrderPartiallyFilledData)
	if partial.ExecutedQty != 50 || partial.LeavesQty != 50 {
		t.Fatalf("unexpected partial data exec=%d leaves=%d", partial.ExecutedQty, partial.LeavesQty)
	}
	if events[4].Type != EventOrderAccepted {
		t.Fatalf("expected remaining order accepted, got %v", events[4].Type)
	}
}

func TestMarketOrderMatch(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         30,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      11,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       101,
		Qty:         50,
	})

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     3,
		UserID:      20,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   2,
		TimeInForce: 1,
		Price:       0,
		Qty:         60,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return len(ev) >= 7
	})

	if events[2].Type != EventTradeCreated || events[3].Type != EventTradeCreated {
		t.Fatalf("expected two trades, got %v/%v", events[2].Type, events[3].Type)
	}
	trade1 := events[2].Data.(*TradeCreatedData)
	trade2 := events[3].Data.(*TradeCreatedData)
	if trade1.Price != 100 || trade1.Qty != 30 {
		t.Fatalf("unexpected trade1 price=%d qty=%d", trade1.Price, trade1.Qty)
	}
	if trade2.Price != 101 || trade2.Qty != 30 {
		t.Fatalf("unexpected trade2 price=%d qty=%d", trade2.Price, trade2.Qty)
	}
	if events[4].Type != EventOrderFilled || events[5].Type != EventOrderPartiallyFilled {
		t.Fatalf("expected maker updates, got %v/%v", events[4].Type, events[5].Type)
	}
	if events[6].Type != EventOrderFilled {
		t.Fatalf("expected taker filled, got %v", events[6].Type)
	}
}

func TestSelfTradeProtection(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         10,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      11,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         5,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     3,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         5,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return len(ev) >= 5
	})
	if events[2].Type != EventTradeCreated {
		t.Fatalf("expected trade event, got %v", events[2].Type)
	}
	trade := events[2].Data.(*TradeCreatedData)
	if trade.MakerUserID != 11 || trade.TakerUserID != 10 {
		t.Fatalf("unexpected trade users maker=%d taker=%d", trade.MakerUserID, trade.TakerUserID)
	}

	bids, asks := engine.Depth(1)
	if len(bids) != 0 || len(asks) != 1 {
		t.Fatalf("expected only asks to have depth, bids=%d asks=%d", len(bids), len(asks))
	}
	if asks[0].Qty != 10 {
		t.Fatalf("unexpected ask quantity=%d", asks[0].Qty)
	}
}

func TestPriceTimePriority(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       101,
		Qty:         10,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      11,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         10,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     3,
		UserID:      12,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         10,
	})

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     4,
		UserID:      20,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 1,
		Price:       101,
		Qty:         25,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return len(ev) >= 7
	})

	trade1 := events[3].Data.(*TradeCreatedData)
	trade2 := events[4].Data.(*TradeCreatedData)
	trade3 := events[5].Data.(*TradeCreatedData)
	if trade1.MakerOrderID != 2 || trade2.MakerOrderID != 3 || trade3.MakerOrderID != 1 {
		t.Fatalf("unexpected maker order priority: %d, %d, %d", trade1.MakerOrderID, trade2.MakerOrderID, trade3.MakerOrderID)
	}
	if trade1.Price != 100 || trade2.Price != 100 || trade3.Price != 101 {
		t.Fatalf("unexpected trade prices: %d, %d, %d", trade1.Price, trade2.Price, trade3.Price)
	}
}

func TestLargeOrderPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	engine := newTestEngine()
	defer engine.Stop()

	start := time.Now()
	totalOrders := 5000
	for i := 0; i < totalOrders; i++ {
		submitOrFail(t, engine, &Command{
			Type:        CmdNewOrder,
			OrderID:     int64(i + 1),
			UserID:      int64(1000 + i),
			Symbol:      "BTCUSDT",
			Side:        orderbook.SideSell,
			OrderType:   1,
			TimeInForce: 1,
			Price:       100 + int64(i%10),
			Qty:         1,
		})
	}

	events := collectUntil(t, engine, 5*time.Second, func(ev []*Event) bool {
		return len(ev) >= totalOrders
	})
	if len(events) < totalOrders {
		t.Fatalf("expected %d accepted events, got %d", totalOrders, len(events))
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("performance test exceeded time limit")
	}
}

func TestCancelOrderSuccess(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         10,
	})
	submitOrFail(t, engine, &Command{
		Type:          CmdCancelOrder,
		OrderID:       1,
		ClientOrderID: "c-1",
		UserID:        10,
		Symbol:        "BTCUSDT",
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return findEvent(ev, EventOrderCanceled) != nil
	})
	canceled := findEvent(events, EventOrderCanceled)
	if canceled == nil {
		t.Fatal("expected cancel event")
	}
	data := canceled.Data.(*OrderCanceledData)
	if data.Reason != "USER_CANCELED" || data.LeavesQty != 10 {
		t.Fatalf("unexpected cancel data reason=%s leaves=%d", data.Reason, data.LeavesQty)
	}
}

func TestCancelOrderNotFound(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:          CmdCancelOrder,
		OrderID:       999,
		ClientOrderID: "c-999",
		UserID:        10,
		Symbol:        "BTCUSDT",
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return findEvent(ev, EventOrderRejected) != nil
	})
	rejected := findEvent(events, EventOrderRejected)
	if rejected == nil {
		t.Fatal("expected reject event")
	}
	data := rejected.Data.(*OrderRejectedData)
	if data.Reason != "ORDER_NOT_FOUND" {
		t.Fatalf("unexpected reject reason=%s", data.Reason)
	}
}

func TestPostOnlyReject(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         5,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      20,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 4,
		Price:       100,
		Qty:         5,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return findEvent(ev, EventOrderRejected) != nil
	})
	rejected := findEvent(events, EventOrderRejected)
	if rejected == nil {
		t.Fatal("expected reject event")
	}
	data := rejected.Data.(*OrderRejectedData)
	if data.Reason != "POST_ONLY_REJECTED" {
		t.Fatalf("unexpected reject reason=%s", data.Reason)
	}
}

func TestIOCPartialCancel(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideSell,
		OrderType:   1,
		TimeInForce: 1,
		Price:       100,
		Qty:         50,
	})
	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     2,
		UserID:      20,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 2,
		Price:       100,
		Qty:         100,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return findEvent(ev, EventOrderCanceled) != nil
	})
	canceled := findEvent(events, EventOrderCanceled)
	if canceled == nil {
		t.Fatal("expected cancel event")
	}
	data := canceled.Data.(*OrderCanceledData)
	if data.Reason != "IOC_EXPIRED" || data.LeavesQty != 50 {
		t.Fatalf("unexpected cancel data reason=%s leaves=%d", data.Reason, data.LeavesQty)
	}
}

func TestFOKNoLiquidity(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   1,
		TimeInForce: 3,
		Price:       100,
		Qty:         10,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return findEvent(ev, EventOrderRejected) != nil
	})
	rejected := findEvent(events, EventOrderRejected)
	if rejected == nil {
		t.Fatal("expected reject event")
	}
	data := rejected.Data.(*OrderRejectedData)
	if data.Reason != "NO_LIQUIDITY" {
		t.Fatalf("unexpected reject reason=%s", data.Reason)
	}
}

func TestMarketOrderNoLiquidity(t *testing.T) {
	engine := newTestEngine()
	defer engine.Stop()

	submitOrFail(t, engine, &Command{
		Type:        CmdNewOrder,
		OrderID:     1,
		UserID:      10,
		Symbol:      "BTCUSDT",
		Side:        orderbook.SideBuy,
		OrderType:   2,
		TimeInForce: 1,
		Price:       0,
		Qty:         10,
	})

	events := collectUntil(t, engine, 2*time.Second, func(ev []*Event) bool {
		return findEvent(ev, EventOrderRejected) != nil
	})
	rejected := findEvent(events, EventOrderRejected)
	if rejected == nil {
		t.Fatal("expected reject event")
	}
	data := rejected.Data.(*OrderRejectedData)
	if data.Reason != "NO_LIQUIDITY" {
		t.Fatalf("unexpected reject reason=%s", data.Reason)
	}
}
