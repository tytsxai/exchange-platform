package engine

import (
	"testing"

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
