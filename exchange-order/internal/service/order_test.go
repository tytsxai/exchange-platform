package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	commondecimal "github.com/exchange/common/pkg/decimal"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/repository"
	"github.com/redis/go-redis/v9"
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
		MaxQty:      "10.0",
		MinNotional: "10.0",
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
	if parseTIF("UNKNOWN") != 1 {
		t.Fatal("expected default GTC=1 for unknown")
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
	if tifToString(0) != "GTC" {
		t.Fatal("expected default GTC")
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

type mockOrderStore struct {
	cfg             *repository.SymbolConfig
	created         bool
	createdOrder    *repository.Order
	getOrderByError error
	existingOrder   *repository.Order
	createErr       error
	createCalls     int
}

type cancelOrderStore struct {
	order         *repository.Order
	getOrderErr   error
	byClient      bool
	lastOpenLimit int
	lastListLimit int
	lastStart     int64
	lastEnd       int64
	symbolConfigs []*repository.SymbolConfig
}

func (c *cancelOrderStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	return nil, nil
}

func (c *cancelOrderStore) GetOrderByClientID(_ context.Context, _ int64, _ string) (*repository.Order, error) {
	if c.byClient {
		if c.getOrderErr != nil {
			return nil, c.getOrderErr
		}
		return c.order, nil
	}
	return nil, repository.ErrOrderNotFound
}

func (c *cancelOrderStore) CreateOrder(_ context.Context, _ *repository.Order) error {
	return nil
}

func (c *cancelOrderStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	if c.getOrderErr != nil {
		return nil, c.getOrderErr
	}
	return c.order, nil
}

func (c *cancelOrderStore) ListOpenOrders(_ context.Context, _ int64, _ string, limit int) ([]*repository.Order, error) {
	c.lastOpenLimit = limit
	return nil, nil
}

func (c *cancelOrderStore) ListOrders(_ context.Context, _ int64, _ string, startTime, endTime int64, limit int) ([]*repository.Order, error) {
	c.lastListLimit = limit
	c.lastStart = startTime
	c.lastEnd = endTime
	return nil, nil
}

func (c *cancelOrderStore) ListSymbolConfigs(_ context.Context) ([]*repository.SymbolConfig, error) {
	return c.symbolConfigs, nil
}

func (m *mockOrderStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	if m.cfg == nil {
		return nil, repository.ErrOrderNotFound
	}
	return m.cfg, nil
}

func (m *mockOrderStore) GetOrderByClientID(_ context.Context, _ int64, _ string) (*repository.Order, error) {
	if m.existingOrder != nil {
		return m.existingOrder, nil
	}
	if m.getOrderByError != nil {
		return nil, m.getOrderByError
	}
	return nil, repository.ErrOrderNotFound
}

func (m *mockOrderStore) CreateOrder(_ context.Context, order *repository.Order) error {
	m.createCalls++
	m.created = true
	m.createdOrder = order
	return m.createErr
}

func (m *mockOrderStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	return nil, repository.ErrOrderNotFound
}

func (m *mockOrderStore) ListOpenOrders(_ context.Context, _ int64, _ string, _ int) ([]*repository.Order, error) {
	return nil, nil
}

func (m *mockOrderStore) ListOrders(_ context.Context, _ int64, _ string, _ int64, _ int64, _ int) ([]*repository.Order, error) {
	return nil, nil
}

func (m *mockOrderStore) ListSymbolConfigs(_ context.Context) ([]*repository.SymbolConfig, error) {
	return nil, nil
}

type mockIDGen struct{}

func (g *mockIDGen) NextID() int64 {
	return 1
}

func TestPriceCreateOrder_OutOfRange(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:         "BTCUSDT",
			BaseAsset:      "BTC",
			QuoteAsset:     "USDT",
			MinQty:         "0.001",
			MaxQty:         "10.0",
			MinNotional:    "10.0",
			PriceTick:      "0.01",
			QtyStep:        "0.001",
			PriceLimitRate: "0.05",
			Status:         1,
		},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{
		price: int64(100 * 1e8),
	}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", validator, nil, nil)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(106 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "PRICE_OUT_OF_RANGE" {
		t.Fatalf("expected PRICE_OUT_OF_RANGE, got %s", resp.ErrorCode)
	}
	if store.created {
		t.Fatal("expected no order created when price out of range")
	}
}

func TestCreateOrder_SymbolNotFound(t *testing.T) {
	store := &mockOrderStore{}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "SYMBOL_NOT_FOUND" {
		t.Fatalf("expected SYMBOL_NOT_FOUND, got %s", resp.ErrorCode)
	}
}

func TestCreateOrder_SymbolNotTrading(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      0,
		},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "SYMBOL_NOT_TRADING" {
		t.Fatalf("expected SYMBOL_NOT_TRADING, got %s", resp.ErrorCode)
	}
}

func TestCreateOrder_IdempotentClientID(t *testing.T) {
	existing := &repository.Order{OrderID: 99}
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      1,
		},
		existingOrder: existing,
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:        1,
		Symbol:        "BTCUSDT",
		Side:          "BUY",
		Type:          "LIMIT",
		Price:         int64(100 * 1e8),
		Quantity:      int64(1 * 1e8),
		ClientOrderID: "client-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Order != existing {
		t.Fatal("expected existing order returned")
	}
}

func TestCreateOrder_ValidationErrors(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      1,
		},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	tests := []struct {
		name      string
		req       *CreateOrderRequest
		errorCode string
	}{
		{
			name: "qty too small",
			req: &CreateOrderRequest{
				UserID:   1,
				Symbol:   "BTCUSDT",
				Side:     "BUY",
				Type:     "LIMIT",
				Price:    int64(100 * 1e8),
				Quantity: int64(0.0009 * 1e8),
			},
			errorCode: "QTY_TOO_SMALL",
		},
		{
			name: "qty step invalid",
			req: &CreateOrderRequest{
				UserID:   1,
				Symbol:   "BTCUSDT",
				Side:     "BUY",
				Type:     "LIMIT",
				Price:    int64(100 * 1e8),
				Quantity: int64(0.0015 * 1e8),
			},
			errorCode: "INVALID_QUANTITY",
		},
		{
			name: "price tick invalid",
			req: &CreateOrderRequest{
				UserID:   1,
				Symbol:   "BTCUSDT",
				Side:     "BUY",
				Type:     "LIMIT",
				Price:    int64(100.005 * 1e8),
				Quantity: int64(0.01 * 1e8),
			},
			errorCode: "INVALID_PRICE",
		},
		{
			name: "notional too small",
			req: &CreateOrderRequest{
				UserID:   1,
				Symbol:   "BTCUSDT",
				Side:     "BUY",
				Type:     "LIMIT",
				Price:    int64(100 * 1e8),
				Quantity: int64(0.05 * 1e8),
			},
			errorCode: "NOTIONAL_TOO_SMALL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store.created = false
			resp, err := svc.CreateOrder(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.ErrorCode != tc.errorCode {
				t.Fatalf("expected %s, got %s", tc.errorCode, resp.ErrorCode)
			}
			if store.created {
				t.Fatal("expected no order created on validation error")
			}
		})
	}
}

func TestCreateOrder_FreezeError(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      1,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	clearingClient := client.NewClearingClient(server.URL, "internal-token")
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, clearingClient, nil)

	if _, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	}); err == nil {
		t.Fatal("expected freeze error")
	}
}

func TestCreateOrder_FreezeRejected(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      1,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := client.FreezeResponse{Success: false, ErrorCode: "INSUFFICIENT_BALANCE"}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	clearingClient := client.NewClearingClient(server.URL, "internal-token")
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, clearingClient, nil)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "INSUFFICIENT_BALANCE" {
		t.Fatalf("expected INSUFFICIENT_BALANCE, got %s", resp.ErrorCode)
	}
	if store.created {
		t.Fatal("expected no order created when freeze rejected")
	}
}

func TestCreateOrder_SendToMatchingError(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      1,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := client.FreezeResponse{Success: true}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	addr := mr.Addr()
	mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: addr})
	defer redisClient.Close()

	clearingClient := client.NewClearingClient(server.URL, "internal-token")
	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", nil, clearingClient, nil)

	if _, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	}); err == nil {
		t.Fatal("expected send to matching error")
	}
}

func TestCreateOrder_CreateOrderFailsDoesNotSend(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:      "BTCUSDT",
			BaseAsset:   "BTC",
			QuoteAsset:  "USDT",
			MinQty:      "0.001",
			MaxQty:      "10.0",
			MinNotional: "10.0",
			PriceTick:   "0.01",
			QtyStep:     "0.001",
			Status:      1,
		},
		createErr: errors.New("db error"),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := client.FreezeResponse{Success: true}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	clearingClient := client.NewClearingClient(server.URL, "internal-token")
	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", nil, clearingClient, nil)

	if _, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	}); err == nil {
		t.Fatal("expected create order error")
	}

	if store.createCalls != 1 {
		t.Fatalf("expected create order called once, got %d", store.createCalls)
	}
	if entries, err := redisClient.XLen(context.Background(), "orders").Result(); err != nil {
		t.Fatalf("xlen: %v", err)
	} else if entries != 0 {
		t.Fatalf("expected no matching messages, got %d", entries)
	}
}

func TestCancelOrder_InvalidParam(t *testing.T) {
	svc := NewOrderService(&cancelOrderStore{}, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "INVALID_PARAM" {
		t.Fatalf("expected INVALID_PARAM, got %s", resp.ErrorCode)
	}
}

func TestCancelOrder_OrderNotFound(t *testing.T) {
	store := &cancelOrderStore{getOrderErr: repository.ErrOrderNotFound}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1, OrderID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "ORDER_NOT_FOUND" {
		t.Fatalf("expected ORDER_NOT_FOUND, got %s", resp.ErrorCode)
	}
}

func TestCancelOrder_WrongUser(t *testing.T) {
	store := &cancelOrderStore{
		order: &repository.Order{OrderID: 10, UserID: 2, Status: repository.StatusNew},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1, OrderID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "ORDER_NOT_FOUND" {
		t.Fatalf("expected ORDER_NOT_FOUND, got %s", resp.ErrorCode)
	}
}

func TestCancelOrder_AlreadyFilled(t *testing.T) {
	store := &cancelOrderStore{
		order: &repository.Order{OrderID: 10, UserID: 1, Status: repository.StatusFilled},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1, OrderID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "ORDER_ALREADY_FILLED" {
		t.Fatalf("expected ORDER_ALREADY_FILLED, got %s", resp.ErrorCode)
	}
}

func TestCancelOrder_Idempotent(t *testing.T) {
	store := &cancelOrderStore{
		order: &repository.Order{OrderID: 10, UserID: 1, Status: repository.StatusCanceled},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1, OrderID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Order == nil || resp.Order.Status != repository.StatusCanceled {
		t.Fatal("expected canceled order response")
	}
}

func TestCancelOrder_SendCancel(t *testing.T) {
	store := &cancelOrderStore{
		order: &repository.Order{OrderID: 10, UserID: 1, Status: repository.StatusNew},
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", nil, nil, nil)
	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1, OrderID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Order == nil {
		t.Fatal("expected order response")
	}
}

func TestCancelOrder_ByClientID(t *testing.T) {
	store := &cancelOrderStore{
		order:    &repository.Order{OrderID: 11, UserID: 1, Status: repository.StatusNew},
		byClient: true,
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", nil, nil, nil)
	resp, err := svc.CancelOrder(context.Background(), &CancelOrderRequest{UserID: 1, ClientOrderID: "c1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Order == nil || resp.Order.OrderID != 11 {
		t.Fatal("expected order from client ID")
	}
}

func TestGetOrderAccess(t *testing.T) {
	store := &cancelOrderStore{
		order: &repository.Order{OrderID: 10, UserID: 1},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	if _, err := svc.GetOrder(context.Background(), 2, 10); !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("expected not found for wrong user, got %v", err)
	}
	if order, err := svc.GetOrder(context.Background(), 1, 10); err != nil || order == nil {
		t.Fatalf("expected order, got %v", err)
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	store := &cancelOrderStore{getOrderErr: repository.ErrOrderNotFound}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	if _, err := svc.GetOrder(context.Background(), 1, 10); err == nil {
		t.Fatal("expected error when order not found")
	}
}

func TestListOpenOrdersLimitDefault(t *testing.T) {
	store := &cancelOrderStore{}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	if _, err := svc.ListOpenOrders(context.Background(), 1, "BTCUSDT", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.lastOpenLimit != 100 {
		t.Fatalf("expected default limit 100, got %d", store.lastOpenLimit)
	}
}

func TestListOrdersDefaults(t *testing.T) {
	store := &cancelOrderStore{}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	if _, err := svc.ListOrders(context.Background(), 1, "BTCUSDT", 0, 0, 2000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.lastListLimit != 500 {
		t.Fatalf("expected default limit 500, got %d", store.lastListLimit)
	}
	if store.lastEnd == 0 || store.lastStart == 0 {
		t.Fatal("expected default start/end time set")
	}
	if store.lastEnd-store.lastStart != int64(7*24*3600*1000) {
		t.Fatalf("expected 7 days window, got %d", store.lastEnd-store.lastStart)
	}
}

func TestGetExchangeInfo(t *testing.T) {
	store := &cancelOrderStore{
		symbolConfigs: []*repository.SymbolConfig{{Symbol: "BTCUSDT"}},
	}
	svc := NewOrderService(store, nil, &mockIDGen{}, "orders", nil, nil, nil)

	infos, err := svc.GetExchangeInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 1 || infos[0].Symbol != "BTCUSDT" {
		t.Fatalf("unexpected exchange info: %+v", infos)
	}
}

func TestPriceCreateOrder_InRange(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:         "BTCUSDT",
			BaseAsset:      "BTC",
			QuoteAsset:     "USDT",
			MinQty:         "0.001",
			MaxQty:         "10.0",
			MinNotional:    "10.0",
			PriceTick:      "0.01",
			QtyStep:        "0.001",
			PriceLimitRate: "0.05",
			Status:         1,
		},
	}

	validator := NewPriceValidator(store, &mockMatchingClient{
		price: int64(100 * 1e8),
	}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	redisClient, clearingClient, cleanup := setupOrderDependencies(t)
	defer cleanup()

	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", validator, clearingClient, nil)
	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(104 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected no error code, got %s", resp.ErrorCode)
	}
	if !store.created {
		t.Fatal("expected order created when price in range")
	}
}

func TestPriceCreateOrder_BoundaryValues(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:         "BTCUSDT",
			BaseAsset:      "BTC",
			QuoteAsset:     "USDT",
			MinQty:         "0.001",
			MaxQty:         "10.0",
			MinNotional:    "10.0",
			PriceTick:      "0.01",
			QtyStep:        "0.001",
			PriceLimitRate: "0.05",
			Status:         1,
		},
	}

	validator := NewPriceValidator(store, &mockMatchingClient{
		price: int64(100 * 1e8),
	}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	redisClient, clearingClient, cleanup := setupOrderDependencies(t)
	defer cleanup()

	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", validator, clearingClient, nil)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(105 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected no error code, got %s", resp.ErrorCode)
	}

	resp, err = svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "SELL",
		Type:     "LIMIT",
		Price:    int64(95 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected no error code, got %s", resp.ErrorCode)
	}
}

func TestPriceCreateOrder_DisabledSkipsCheck(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:         "BTCUSDT",
			BaseAsset:      "BTC",
			QuoteAsset:     "USDT",
			MinQty:         "0.001",
			MaxQty:         "10.0",
			MinNotional:    "10.0",
			PriceTick:      "0.01",
			QtyStep:        "0.001",
			PriceLimitRate: "0.05",
			Status:         1,
		},
	}

	matching := &mockMatchingClient{price: int64(100 * 1e8)}
	validator := NewPriceValidator(store, matching, PriceValidatorConfig{
		Enabled:          false,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	redisClient, clearingClient, cleanup := setupOrderDependencies(t)
	defer cleanup()

	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", validator, clearingClient, nil)
	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:   1,
		Symbol:   "BTCUSDT",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    int64(100 * 1e8),
		Quantity: int64(1 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected no error code, got %s", resp.ErrorCode)
	}
	if !store.created {
		t.Fatal("expected order created when price protection disabled")
	}
	if matching.calls != 0 {
		t.Fatalf("expected matching not called when disabled, got %d", matching.calls)
	}
}

func TestPriceCreateOrder_MarketSkipsCheck(t *testing.T) {
	store := &mockOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:         "BTCUSDT",
			BaseAsset:      "BTC",
			QuoteAsset:     "USDT",
			MinQty:         "0.001",
			MaxQty:         "10.0",
			MinNotional:    "10.0",
			PriceTick:      "0.01",
			QtyStep:        "0.001",
			PriceLimitRate: "0.05",
			Status:         1,
		},
	}

	matching := &mockMatchingClient{price: int64(100 * 1e8)}
	validator := NewPriceValidator(store, matching, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	redisClient, clearingClient, cleanup := setupOrderDependencies(t)
	defer cleanup()

	svc := NewOrderService(store, redisClient, &mockIDGen{}, "orders", validator, clearingClient, nil)
	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		UserID:        1,
		Symbol:        "BTCUSDT",
		Side:          "BUY",
		Type:          "MARKET",
		Quantity:      int64(1 * 1e8),
		QuoteOrderQty: int64(100 * 1e8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected no error code, got %s", resp.ErrorCode)
	}
	if !store.created {
		t.Fatal("expected order created when market order")
	}
	if matching.calls != 1 {
		t.Fatalf("expected matching called once for market order, got %d", matching.calls)
	}
}

func setupOrderDependencies(t *testing.T) (*redis.Client, *client.ClearingClient, func()) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/freeze" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := client.FreezeResponse{Success: true}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	clearingClient := client.NewClearingClient(server.URL, "internal-token")
	cleanup := func() {
		redisClient.Close()
		mr.Close()
		server.Close()
	}

	return redisClient, clearingClient, cleanup
}
