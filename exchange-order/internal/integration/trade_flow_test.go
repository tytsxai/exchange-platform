package integration

import (
	"context"
	"testing"
	"time"
)

type testEnv struct {
	ctx      context.Context
	cancel   context.CancelFunc
	orderSvc *orderService
	matching MatchingService
	clearing ClearingService
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	env := &testEnv{
		ctx:     ctx,
		cancel: cancel,
	}

	env.matching = &mockMatchingService{}
	env.clearing = &mockClearingService{}
	env.orderSvc = &orderService{matching: env.matching, clearing: env.clearing}

	return env
}

func teardownTestEnv(t *testing.T, env *testEnv) {
	t.Helper()
	if env != nil && env.cancel != nil {
		env.cancel()
	}
}

type MatchingService interface {
	SubmitOrder(ctx context.Context, req *SubmitOrderReq) (*SubmitOrderResp, error)
	CancelOrder(ctx context.Context, req *CancelOrderReq) (*CancelOrderResp, error)
}

type ClearingService interface {
	SettleTrade(ctx context.Context, req *SettleTradeReq) (*SettleTradeResp, error)
}

type mockMatchingService struct {
	SubmitOrderFn func(ctx context.Context, req *SubmitOrderReq) (*SubmitOrderResp, error)
	CancelOrderFn func(ctx context.Context, req *CancelOrderReq) (*CancelOrderResp, error)
}

func (m *mockMatchingService) SubmitOrder(ctx context.Context, req *SubmitOrderReq) (*SubmitOrderResp, error) {
	if m.SubmitOrderFn != nil {
		return m.SubmitOrderFn(ctx, req)
	}
	return &SubmitOrderResp{OrderID: "mock-order-id"}, nil
}

func (m *mockMatchingService) CancelOrder(ctx context.Context, req *CancelOrderReq) (*CancelOrderResp, error) {
	if m.CancelOrderFn != nil {
		return m.CancelOrderFn(ctx, req)
	}
	return &CancelOrderResp{Canceled: true}, nil
}

type mockClearingService struct {
	SettleTradeFn func(ctx context.Context, req *SettleTradeReq) (*SettleTradeResp, error)
}

func (m *mockClearingService) SettleTrade(ctx context.Context, req *SettleTradeReq) (*SettleTradeResp, error) {
	if m.SettleTradeFn != nil {
		return m.SettleTradeFn(ctx, req)
	}
	return &SettleTradeResp{Settled: true}, nil
}

type orderService struct {
	matching MatchingService
	clearing ClearingService
}

func (s *orderService) PlaceOrder(ctx context.Context, symbol string) (string, error) {
	resp, err := s.matching.SubmitOrder(ctx, &SubmitOrderReq{Symbol: symbol})
	if err != nil {
		return "", err
	}
	return resp.OrderID, nil
}

func (s *orderService) Cancel(ctx context.Context, orderID string) error {
	_, err := s.matching.CancelOrder(ctx, &CancelOrderReq{OrderID: orderID})
	return err
}

func (s *orderService) MatchAndSettle(ctx context.Context, orderID string) error {
	_, err := s.clearing.SettleTrade(ctx, &SettleTradeReq{OrderID: orderID})
	return err
}

type (
	SubmitOrderReq   struct{ Symbol string }
	SubmitOrderResp  struct{ OrderID string }
	CancelOrderReq   struct{ OrderID string }
	CancelOrderResp  struct{ Canceled bool }
	SettleTradeReq   struct{ OrderID string }
	SettleTradeResp  struct{ Settled bool }
)

// TestTradeFlow_PlaceOrder 测试下单流程
func TestTradeFlow_PlaceOrder(t *testing.T) {
	env := setupTestEnv(t)
	t.Cleanup(func() { teardownTestEnv(t, env) })

	orderID, err := env.orderSvc.PlaceOrder(env.ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("place order failed: %v", err)
	}
	if orderID == "" {
		t.Fatal("expected non-empty orderID")
	}
}

// TestTradeFlow_CancelOrder 测试撤单流程
func TestTradeFlow_CancelOrder(t *testing.T) {
	env := setupTestEnv(t)
	t.Cleanup(func() { teardownTestEnv(t, env) })

	orderID, err := env.orderSvc.PlaceOrder(env.ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("precondition place order failed: %v", err)
	}
	if err := env.orderSvc.Cancel(env.ctx, orderID); err != nil {
		t.Fatalf("cancel order failed: %v", err)
	}
}

// TestTradeFlow_MatchAndSettle 测试撮合清算流程
func TestTradeFlow_MatchAndSettle(t *testing.T) {
	env := setupTestEnv(t)
	t.Cleanup(func() { teardownTestEnv(t, env) })

	orderID, err := env.orderSvc.PlaceOrder(env.ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("precondition place order failed: %v", err)
	}
	if err := env.orderSvc.MatchAndSettle(env.ctx, orderID); err != nil {
		t.Fatalf("match/settle failed: %v", err)
	}
}
