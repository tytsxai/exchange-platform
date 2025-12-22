package service

import (
	"testing"
)

func TestClearingServiceConstants(t *testing.T) {
	svc := &ClearingService{}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestFreezeRequest_Fields(t *testing.T) {
	req := &FreezeRequest{
		IdempotencyKey: "freeze-key-1",
		UserID:         123,
		Asset:          "USDT",
		Amount:         1000,
		RefType:        "ORDER",
		RefID:          "456",
	}

	if req.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", req.UserID)
	}
	if req.Asset != "USDT" {
		t.Fatalf("expected Asset=USDT, got %s", req.Asset)
	}
	if req.Amount != 1000 {
		t.Fatalf("expected Amount=1000, got %d", req.Amount)
	}
	if req.RefID != "456" {
		t.Fatalf("expected RefID=456, got %s", req.RefID)
	}
}

func TestFreezeResponse_Fields(t *testing.T) {
	resp := &FreezeResponse{
		Success:   true,
		ErrorCode: "",
	}

	if !resp.Success {
		t.Fatal("expected Success=true")
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected empty ErrorCode, got %s", resp.ErrorCode)
	}

	// Test failure case
	resp = &FreezeResponse{
		Success:   false,
		ErrorCode: "INSUFFICIENT_BALANCE",
	}

	if resp.Success {
		t.Fatal("expected Success=false")
	}
	if resp.ErrorCode != "INSUFFICIENT_BALANCE" {
		t.Fatalf("expected ErrorCode=INSUFFICIENT_BALANCE, got %s", resp.ErrorCode)
	}
}

func TestUnfreezeRequest_Fields(t *testing.T) {
	req := &UnfreezeRequest{
		IdempotencyKey: "unfreeze-key-1",
		UserID:         123,
		Asset:          "BTC",
		Amount:         500,
		RefType:        "ORDER",
		RefID:          "789",
	}

	if req.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", req.UserID)
	}
	if req.Asset != "BTC" {
		t.Fatalf("expected Asset=BTC, got %s", req.Asset)
	}
	if req.Amount != 500 {
		t.Fatalf("expected Amount=500, got %d", req.Amount)
	}
}

func TestUnfreezeResponse_Fields(t *testing.T) {
	resp := &UnfreezeResponse{
		Success:   true,
		ErrorCode: "",
	}

	if !resp.Success {
		t.Fatal("expected Success=true")
	}
}

func TestSettleTradeRequest_Fields(t *testing.T) {
	req := &SettleTradeRequest{
		IdempotencyKey:  "settle-key-1",
		TradeID:         "trade-1",
		Symbol:          "BTCUSDT",
		MakerUserID:     100,
		TakerUserID:     200,
		MakerOrderID:    "order-1000",
		TakerOrderID:    "order-2000",
		MakerBaseDelta:  -100,
		MakerQuoteDelta: 5000000,
		TakerBaseDelta:  100,
		TakerQuoteDelta: -5000000,
		MakerFee:        10,
		TakerFee:        20,
		MakerFeeAsset:   "USDT",
		TakerFeeAsset:   "BTC",
		BaseAsset:       "BTC",
		QuoteAsset:      "USDT",
	}

	if req.TradeID != "trade-1" {
		t.Fatalf("expected TradeID=trade-1, got %s", req.TradeID)
	}
	if req.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", req.Symbol)
	}
	if req.MakerUserID != 100 {
		t.Fatalf("expected MakerUserID=100, got %d", req.MakerUserID)
	}
	if req.TakerUserID != 200 {
		t.Fatalf("expected TakerUserID=200, got %d", req.TakerUserID)
	}
	if req.MakerBaseDelta != -100 {
		t.Fatalf("expected MakerBaseDelta=-100, got %d", req.MakerBaseDelta)
	}
}

func TestSettleTradeResponse_Fields(t *testing.T) {
	resp := &SettleTradeResponse{
		Success:   true,
		ErrorCode: "",
	}

	if !resp.Success {
		t.Fatal("expected Success=true")
	}

	resp = &SettleTradeResponse{
		Success:   false,
		ErrorCode: "SETTLEMENT_FAILED",
	}

	if resp.Success {
		t.Fatal("expected Success=false")
	}
	if resp.ErrorCode != "SETTLEMENT_FAILED" {
		t.Fatalf("expected ErrorCode=SETTLEMENT_FAILED, got %s", resp.ErrorCode)
	}
}

func TestIDGeneratorInterface(t *testing.T) {
	var _ IDGenerator = &mockIDGen{}
}

type mockIDGen struct {
	id int64
}

func (m *mockIDGen) NextID() int64 {
	m.id++
	return m.id
}

func TestNewClearingService(t *testing.T) {
	gen := &mockIDGen{}
	svc := NewClearingService(nil, gen)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestMaxInt64(t *testing.T) {
	if maxInt64(10, 20) != 20 {
		t.Fatal("expected maxInt64(10, 20) = 20")
	}
	if maxInt64(20, 10) != 20 {
		t.Fatal("expected maxInt64(20, 10) = 20")
	}
	if maxInt64(-10, 0) != 0 {
		t.Fatal("expected maxInt64(-10, 0) = 0")
	}
}

func TestMinInt64(t *testing.T) {
	if minInt64(10, 20) != 10 {
		t.Fatal("expected minInt64(10, 20) = 10")
	}
	if minInt64(20, 10) != 10 {
		t.Fatal("expected minInt64(20, 10) = 10")
	}
	if minInt64(-10, 0) != -10 {
		t.Fatal("expected minInt64(-10, 0) = -10")
	}
}
