package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/exchange/order/internal/repository"
)

func TestValidateOrder_Boundaries(t *testing.T) {
	s := &OrderService{}
	cfg := &repository.SymbolConfig{
		Symbol:      "BTCUSDT",
		BaseAsset:   "BTC",
		QuoteAsset:  "USDT",
		PriceTick:   "0.01",
		QtyStep:     "0.001",
		MinQty:      "0.001",
		MaxQty:      "10",
		MinNotional: "10",
		Status:      1,
	}

	// qty too small: 0.0009
	req := &CreateOrderRequest{Type: "LIMIT", Price: 100 * 1e8, Quantity: int64(0.0009 * 1e8)}
	if err := s.validateOrder(req, cfg); err == nil || err.Error() != "QTY_TOO_SMALL" {
		t.Fatalf("expected QTY_TOO_SMALL, got %v", err)
	}

	// qty too large: 10.0001
	req = &CreateOrderRequest{Type: "LIMIT", Price: 100 * 1e8, Quantity: int64(10.0001 * 1e8)}
	if err := s.validateOrder(req, cfg); err == nil || err.Error() != "QTY_TOO_LARGE" {
		t.Fatalf("expected QTY_TOO_LARGE, got %v", err)
	}

	// invalid price: 0
	req = &CreateOrderRequest{Type: "LIMIT", Price: 0, Quantity: int64(0.001 * 1e8)}
	if err := s.validateOrder(req, cfg); err == nil || err.Error() != "INVALID_PRICE" {
		t.Fatalf("expected INVALID_PRICE, got %v", err)
	}

	// notional too small: price 100, qty 0.05 => 5 < 10
	req = &CreateOrderRequest{Type: "LIMIT", Price: 100 * 1e8, Quantity: int64(0.05 * 1e8)}
	if err := s.validateOrder(req, cfg); err == nil || err.Error() != "NOTIONAL_TOO_SMALL" {
		t.Fatalf("expected NOTIONAL_TOO_SMALL, got %v", err)
	}

	// market skips price/notional checks (only qty checks)
	req = &CreateOrderRequest{Type: "MARKET", Price: 0, Quantity: int64(0.001 * 1e8)}
	if err := s.validateOrder(req, cfg); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// ok limit
	req = &CreateOrderRequest{Type: "LIMIT", Price: 100 * 1e8, Quantity: int64(0.2 * 1e8)} // notional 20
	if err := s.validateOrder(req, cfg); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestParseSide(t *testing.T) {
	if parseSide("BUY") != repository.SideBuy {
		t.Fatal("expected SideBuy")
	}
	if parseSide("SELL") != repository.SideSell {
		t.Fatal("expected SideSell")
	}
	if parseSide("") != repository.SideBuy {
		t.Fatal("expected default SideBuy")
	}
}

func TestParseType(t *testing.T) {
	if parseType("LIMIT") != repository.TypeLimit {
		t.Fatal("expected TypeLimit")
	}
	if parseType("MARKET") != repository.TypeMarket {
		t.Fatal("expected TypeMarket")
	}
	if parseType("") != repository.TypeLimit {
		t.Fatal("expected default TypeLimit")
	}
}

func TestParseTIF(t *testing.T) {
	if parseTIF("GTC") != 1 {
		t.Fatal("expected GTC=1")
	}
	if parseTIF("IOC") != 2 {
		t.Fatal("expected IOC=2")
	}
	if parseTIF("FOK") != 3 {
		t.Fatal("expected FOK=3")
	}
	if parseTIF("") != 1 {
		t.Fatal("expected default GTC=1")
	}
}

func TestSideToString(t *testing.T) {
	if sideToString(repository.SideBuy) != "BUY" {
		t.Fatal("expected BUY")
	}
	if sideToString(repository.SideSell) != "SELL" {
		t.Fatal("expected SELL")
	}
}

func TestTypeToString(t *testing.T) {
	if typeToString(repository.TypeLimit) != "LIMIT" {
		t.Fatal("expected LIMIT")
	}
	if typeToString(repository.TypeMarket) != "MARKET" {
		t.Fatal("expected MARKET")
	}
}

func TestTifToString(t *testing.T) {
	if tifToString(1) != "GTC" {
		t.Fatal("expected GTC")
	}
	if tifToString(2) != "IOC" {
		t.Fatal("expected IOC")
	}
	if tifToString(3) != "FOK" {
		t.Fatal("expected FOK")
	}
}

func TestCreateOrderResponse_Fields(t *testing.T) {
	order := &repository.Order{
		OrderID:       123,
		ClientOrderID: "test",
		UserID:        456,
		Symbol:        "BTCUSDT",
		Side:          repository.SideBuy,
		Type:          repository.TypeLimit,
		Price:         strconv.FormatInt(100*1e8, 10),
		OrigQty:       strconv.FormatInt(1*1e8, 10),
	}

	resp := &CreateOrderResponse{
		Order: order,
	}

	if resp.Order.OrderID != 123 {
		t.Fatalf("expected OrderID=123, got %d", resp.Order.OrderID)
	}
	if resp.Order.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", resp.Order.Symbol)
	}
}

func TestFreezeBalance_AlwaysSucceeds(t *testing.T) {
	s := &OrderService{}
	ctx := context.Background()

	resp, err := s.freezeBalance(ctx, 1, "USDT", 1000, 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
}
