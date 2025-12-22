package repository

import (
	"testing"
)

func TestOrderConstants(t *testing.T) {
	// Side constants
	if SideBuy != 1 {
		t.Fatalf("expected SideBuy=1, got %d", SideBuy)
	}
	if SideSell != 2 {
		t.Fatalf("expected SideSell=2, got %d", SideSell)
	}

	// Type constants
	if TypeLimit != 1 {
		t.Fatalf("expected TypeLimit=1, got %d", TypeLimit)
	}
	if TypeMarket != 2 {
		t.Fatalf("expected TypeMarket=2, got %d", TypeMarket)
	}

	// Status constants
	if StatusNew != 1 {
		t.Fatalf("expected StatusNew=1, got %d", StatusNew)
	}
	if StatusPartiallyFilled != 2 {
		t.Fatalf("expected StatusPartiallyFilled=2, got %d", StatusPartiallyFilled)
	}
	if StatusFilled != 3 {
		t.Fatalf("expected StatusFilled=3, got %d", StatusFilled)
	}
	if StatusCanceled != 4 {
		t.Fatalf("expected StatusCanceled=4, got %d", StatusCanceled)
	}
	if StatusRejected != 5 {
		t.Fatalf("expected StatusRejected=5, got %d", StatusRejected)
	}
}

func TestOrderStruct(t *testing.T) {
	order := &Order{
		OrderID:            123,
		ClientOrderID:      "test-client-id",
		UserID:             456,
		Symbol:             "BTCUSDT",
		Side:               SideBuy,
		Type:               TypeLimit,
		TimeInForce:        1,
		Price:              "50000",
		StopPrice:          "0",
		OrigQty:            "1",
		ExecutedQty:        "0",
		CumulativeQuoteQty: "0",
		Status:             StatusNew,
		RejectReason:       "",
		CancelReason:       "",
		CreateTimeMs:       1000,
		UpdateTimeMs:       1000,
		TransactTimeMs:     0,
	}

	if order.OrderID != 123 {
		t.Fatalf("expected OrderID=123, got %d", order.OrderID)
	}
	if order.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", order.Symbol)
	}
	if order.Side != SideBuy {
		t.Fatalf("expected Side=SideBuy, got %d", order.Side)
	}
	if order.Type != TypeLimit {
		t.Fatalf("expected Type=TypeLimit, got %d", order.Type)
	}
	if order.Status != StatusNew {
		t.Fatalf("expected Status=StatusNew, got %d", order.Status)
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
		Qty:          1,
		QuoteQty:     50000,
		MakerFee:     10,
		TakerFee:     20,
		TakerSide:    SideBuy,
		TimestampMs:  1000,
	}

	if trade.TradeID != 1 {
		t.Fatalf("expected TradeID=1, got %d", trade.TradeID)
	}
	if trade.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", trade.Symbol)
	}
	if trade.MakerOrderID != 100 {
		t.Fatalf("expected MakerOrderID=100, got %d", trade.MakerOrderID)
	}
	if trade.TakerOrderID != 200 {
		t.Fatalf("expected TakerOrderID=200, got %d", trade.TakerOrderID)
	}
}

func TestSymbolConfigStruct(t *testing.T) {
	cfg := &SymbolConfig{
		Symbol:         "BTCUSDT",
		BaseAsset:      "BTC",
		QuoteAsset:     "USDT",
		PriceTick:      "0.01",
		QtyStep:        "0.001",
		PricePrecision: 2,
		QtyPrecision:   3,
		MinQty:         "0.001",
		MaxQty:         "1000",
		MinNotional:    "10",
		PriceLimitRate: "0.1",
		MakerFeeRate:   "0.001",
		TakerFeeRate:   "0.001",
		Status:         1,
	}

	if cfg.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", cfg.Symbol)
	}
	if cfg.BaseAsset != "BTC" {
		t.Fatalf("expected BaseAsset=BTC, got %s", cfg.BaseAsset)
	}
	if cfg.QuoteAsset != "USDT" {
		t.Fatalf("expected QuoteAsset=USDT, got %s", cfg.QuoteAsset)
	}
	if cfg.PricePrecision != 2 {
		t.Fatalf("expected PricePrecision=2, got %d", cfg.PricePrecision)
	}
	if cfg.QtyPrecision != 3 {
		t.Fatalf("expected QtyPrecision=3, got %d", cfg.QtyPrecision)
	}
}

func TestNullString(t *testing.T) {
	// Test with empty string
	ns := nullString("")
	if ns.Valid {
		t.Fatal("expected Valid=false for empty string")
	}

	// Test with non-empty string
	ns = nullString("test")
	if !ns.Valid {
		t.Fatal("expected Valid=true for non-empty string")
	}
	if ns.String != "test" {
		t.Fatalf("expected String=test, got %s", ns.String)
	}
}

func TestErrOrderNotFound(t *testing.T) {
	if ErrOrderNotFound == nil {
		t.Fatal("ErrOrderNotFound should not be nil")
	}
	if ErrOrderNotFound.Error() != "order not found" {
		t.Fatalf("expected 'order not found', got %s", ErrOrderNotFound.Error())
	}
}

func TestErrDuplicateClientOrderID(t *testing.T) {
	if ErrDuplicateClientOrderID == nil {
		t.Fatal("ErrDuplicateClientOrderID should not be nil")
	}
	if ErrDuplicateClientOrderID.Error() != "duplicate client order id" {
		t.Fatalf("expected 'duplicate client order id', got %s", ErrDuplicateClientOrderID.Error())
	}
}

func TestNewOrderRepository(t *testing.T) {
	repo := NewOrderRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
