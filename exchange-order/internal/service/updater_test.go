package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/repository"
	redismock "github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
)

type fakeOrderStore struct {
	order *repository.Order
	cfg   *repository.SymbolConfig

	cancelCalled bool
	cancelReason string
	cancelID     int64

	updateCalls []updateCall
	addCalls    []addCall
}

type updateCall struct {
	orderID int64
	status  int
	execQty int64
	cumQty  int64
}

type addCall struct {
	orderID int64
	delta   int64
}

func (f *fakeOrderStore) UpdateOrderStatus(_ context.Context, orderID int64, status int, executedQty, cumulativeQuoteQty, _ int64) error {
	f.updateCalls = append(f.updateCalls, updateCall{
		orderID: orderID,
		status:  status,
		execQty: executedQty,
		cumQty:  cumulativeQuoteQty,
	})
	return nil
}

func (f *fakeOrderStore) CancelOrder(_ context.Context, orderID int64, reason string, _ int64) error {
	f.cancelCalled = true
	f.cancelID = orderID
	f.cancelReason = reason
	return nil
}

func (f *fakeOrderStore) RejectOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (f *fakeOrderStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	return f.order, nil
}

func (f *fakeOrderStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	return f.cfg, nil
}

func (f *fakeOrderStore) AddOrderCumulativeQuoteQty(_ context.Context, orderID int64, delta int64, _ int64) error {
	f.addCalls = append(f.addCalls, addCall{orderID: orderID, delta: delta})
	return nil
}

type fakeTradeStore struct {
	saved *repository.Trade
}

func (f *fakeTradeStore) SaveTrade(_ context.Context, trade *repository.Trade) error {
	f.saved = trade
	return nil
}

type errorTradeStore struct {
	err error
}

func (e *errorTradeStore) SaveTrade(_ context.Context, _ *repository.Trade) error {
	return e.err
}

type addQtyErrorStore struct {
	cfg *repository.SymbolConfig
}

func (a *addQtyErrorStore) UpdateOrderStatus(_ context.Context, _ int64, _ int, _ int64, _ int64, _ int64) error {
	return nil
}

func (a *addQtyErrorStore) CancelOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (a *addQtyErrorStore) RejectOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (a *addQtyErrorStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	return nil, nil
}

func (a *addQtyErrorStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	return a.cfg, nil
}

func (a *addQtyErrorStore) AddOrderCumulativeQuoteQty(_ context.Context, _ int64, _ int64, _ int64) error {
	return errors.New("update failed")
}

type errorSymbolStore struct {
	order *repository.Order
	err   error
}

func (e *errorSymbolStore) UpdateOrderStatus(_ context.Context, _ int64, _ int, _ int64, _ int64, _ int64) error {
	return nil
}

func (e *errorSymbolStore) CancelOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (e *errorSymbolStore) RejectOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (e *errorSymbolStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	return e.order, nil
}

func (e *errorSymbolStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	return nil, e.err
}

func (e *errorSymbolStore) AddOrderCumulativeQuoteQty(_ context.Context, _ int64, _ int64, _ int64) error {
	return nil
}

type cancelErrStore struct {
	order *repository.Order
	cfg   *repository.SymbolConfig
}

func (c *cancelErrStore) UpdateOrderStatus(_ context.Context, _ int64, _ int, _ int64, _ int64, _ int64) error {
	return nil
}

func (c *cancelErrStore) CancelOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return repository.ErrOrderNotFound
}

func (c *cancelErrStore) RejectOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (c *cancelErrStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	return c.order, nil
}

func (c *cancelErrStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	return c.cfg, nil
}

func (c *cancelErrStore) AddOrderCumulativeQuoteQty(_ context.Context, _ int64, _ int64, _ int64) error {
	return nil
}

type orderErrStore struct {
	err error
}

func (o *orderErrStore) UpdateOrderStatus(_ context.Context, _ int64, _ int, _ int64, _ int64, _ int64) error {
	return nil
}

func (o *orderErrStore) CancelOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (o *orderErrStore) RejectOrder(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (o *orderErrStore) GetOrder(_ context.Context, _ int64) (*repository.Order, error) {
	return nil, o.err
}

func (o *orderErrStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	return nil, nil
}

func (o *orderErrStore) AddOrderCumulativeQuoteQty(_ context.Context, _ int64, _ int64, _ int64) error {
	return nil
}

type fakeUnfreezer struct {
	called bool
	asset  string
	amount int64
}

func (f *fakeUnfreezer) UnfreezeBalance(_ context.Context, _ int64, asset string, amount int64, _ string) (*client.UnfreezeResponse, error) {
	f.called = true
	f.asset = asset
	f.amount = amount
	return &client.UnfreezeResponse{Success: true}, nil
}

type failingUnfreezer struct {
	resp *client.UnfreezeResponse
	err  error
}

func (f *failingUnfreezer) UnfreezeBalance(_ context.Context, _ int64, _ string, _ int64, _ string) (*client.UnfreezeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestOrderUpdater_ConsumeOnce_OrderCanceled(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()

	store := &fakeOrderStore{
		order: &repository.Order{
			OrderID: 1,
			UserID:  10,
			Symbol:  "BTCUSDT",
			Side:    repository.SideBuy,
			Price:   "100000000",
			OrigQty: "200000000",
		},
		cfg: &repository.SymbolConfig{
			Symbol:     "BTCUSDT",
			BaseAsset:  "BTC",
			QuoteAsset: "USDT",
		},
	}
	unfreezer := &fakeUnfreezer{}

	updater := NewOrderUpdater(redisClient, store, &fakeTradeStore{}, unfreezer, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "order-updater-group",
		Consumer:    "order-updater-1",
	})

	payload := MatchingEvent{
		Type:   "ORDER_CANCELED",
		Symbol: "BTCUSDT",
		Data: mustJSON(t, OrderCanceledData{
			OrderID:   1,
			UserID:    10,
			LeavesQty: 2 * 1e8,
			Reason:    "USER_CANCELED",
		}),
	}
	raw, _ := json.Marshal(payload)

	mock.ExpectXReadGroup(&redis.XReadGroupArgs{
		Group:    "order-updater-group",
		Consumer: "order-updater-1",
		Streams:  []string{"matching:events", ">"},
		Count:    100,
		Block:    time.Second,
	}).SetVal([]redis.XStream{
		{
			Stream: "matching:events",
			Messages: []redis.XMessage{
				{
					ID:     "1-0",
					Values: map[string]interface{}{"data": string(raw)},
				},
			},
		},
	})
	mock.ExpectXAck("matching:events", "order-updater-group", "1-0").SetVal(1)

	if err := updater.consumeOnce(context.Background()); err != nil {
		t.Fatalf("consume once: %v", err)
	}

	if !store.cancelCalled {
		t.Fatal("expected cancel order to be called")
	}
	if !unfreezer.called {
		t.Fatal("expected unfreeze to be called")
	}
	if unfreezer.asset != "USDT" || unfreezer.amount != 2*1e8 {
		t.Fatalf("unexpected unfreeze: %s %d", unfreezer.asset, unfreezer.amount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestOrderUpdater_ConsumeOnce_TradeCreated(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()

	store := &fakeOrderStore{
		cfg: &repository.SymbolConfig{
			Symbol:     "BTCUSDT",
			BaseAsset:  "BTC",
			QuoteAsset: "USDT",
		},
	}
	trades := &fakeTradeStore{}
	unfreezer := &fakeUnfreezer{}

	updater := NewOrderUpdater(redisClient, store, trades, unfreezer, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "order-updater-group",
		Consumer:    "order-updater-1",
	})

	payload := MatchingEvent{
		Type:      "TRADE_CREATED",
		Symbol:    "BTCUSDT",
		Timestamp: time.Now().UnixNano(),
		Data: mustJSON(t, TradeData{
			TradeID:      100,
			MakerOrderID: 1,
			TakerOrderID: 2,
			MakerUserID:  10,
			TakerUserID:  11,
			Price:        100 * 1e8,
			Qty:          2 * 1e8,
			TakerSide:    repository.SideBuy,
		}),
	}
	raw, _ := json.Marshal(payload)

	mock.ExpectXReadGroup(&redis.XReadGroupArgs{
		Group:    "order-updater-group",
		Consumer: "order-updater-1",
		Streams:  []string{"matching:events", ">"},
		Count:    100,
		Block:    time.Second,
	}).SetVal([]redis.XStream{
		{
			Stream: "matching:events",
			Messages: []redis.XMessage{
				{
					ID:     "2-0",
					Values: map[string]interface{}{"data": string(raw)},
				},
			},
		},
	})
	mock.ExpectXAck("matching:events", "order-updater-group", "2-0").SetVal(1)

	if err := updater.consumeOnce(context.Background()); err != nil {
		t.Fatalf("consume once: %v", err)
	}

	if trades.saved == nil || trades.saved.TradeID != 100 {
		t.Fatal("expected trade to be saved")
	}
	if len(store.addCalls) != 2 {
		t.Fatalf("expected 2 cumulative updates, got %d", len(store.addCalls))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestOrderUpdater_StartError(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	mock.ExpectXGroupCreateMkStream("matching:events", "group", "0").SetErr(errors.New("boom"))

	updater := NewOrderUpdater(redisClient, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "group",
		Consumer:    "consumer",
	})

	if err := updater.Start(context.Background()); err == nil {
		t.Fatal("expected start error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestOrderUpdater_ConsumeOnce_Errors(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	updater := NewOrderUpdater(redisClient, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "group",
		Consumer:    "consumer",
	})

	mock.ExpectXReadGroup(&redis.XReadGroupArgs{
		Group:    "group",
		Consumer: "consumer",
		Streams:  []string{"matching:events", ">"},
		Count:    100,
		Block:    time.Second,
	}).SetErr(redis.Nil)

	if err := updater.consumeOnce(context.Background()); err != nil {
		t.Fatalf("expected nil on redis.Nil, got %v", err)
	}

	mock.ExpectXReadGroup(&redis.XReadGroupArgs{
		Group:    "group",
		Consumer: "consumer",
		Streams:  []string{"matching:events", ">"},
		Count:    100,
		Block:    time.Second,
	}).SetErr(errors.New("read failed"))

	if err := updater.consumeOnce(context.Background()); err == nil {
		t.Fatal("expected error on read failure")
	}
}

func TestOrderUpdater_ConsumeOnce_ProcessError(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	updater := NewOrderUpdater(redisClient, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "group",
		Consumer:    "consumer",
	})

	mock.ExpectXReadGroup(&redis.XReadGroupArgs{
		Group:    "group",
		Consumer: "consumer",
		Streams:  []string{"matching:events", ">"},
		Count:    100,
		Block:    time.Second,
	}).SetVal([]redis.XStream{
		{
			Stream: "matching:events",
			Messages: []redis.XMessage{
				{
					ID:     "1-0",
					Values: map[string]interface{}{"data": "{"},
				},
			},
		},
	})

	if err := updater.consumeOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOrderUpdater_ProcessMessage_Invalid(t *testing.T) {
	updater := NewOrderUpdater(nil, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	msg := redis.XMessage{Values: map[string]interface{}{"data": 123}}
	if err := updater.processMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg = redis.XMessage{Values: map[string]interface{}{"data": "{"}}
	if err := updater.processMessage(context.Background(), msg); err == nil {
		t.Fatal("expected unmarshal error")
	}

	payload := MatchingEvent{Type: "UNKNOWN"}
	raw, _ := json.Marshal(payload)
	msg = redis.XMessage{Values: map[string]interface{}{"data": string(raw)}}
	if err := updater.processMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOrderUpdater_ProcessMessage_StatusHandlers(t *testing.T) {
	store := &fakeOrderStore{
		order: &repository.Order{
			OrderID:            1,
			CumulativeQuoteQty: "100",
		},
	}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	events := []MatchingEvent{
		{Type: "ORDER_ACCEPTED", Data: mustJSON(t, OrderAcceptedData{OrderID: 1})},
		{Type: "ORDER_PARTIALLY_FILLED", Data: mustJSON(t, OrderPartiallyFilledData{OrderID: 1, ExecutedQty: 2})},
		{Type: "ORDER_FILLED", Data: mustJSON(t, OrderFilledData{OrderID: 1, ExecutedQty: 3})},
	}

	for _, evt := range events {
		raw, _ := json.Marshal(evt)
		msg := redis.XMessage{Values: map[string]interface{}{"data": string(raw)}}
		if err := updater.processMessage(context.Background(), msg); err != nil {
			t.Fatalf("process message: %v", err)
		}
	}

	if len(store.updateCalls) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(store.updateCalls))
	}
}

func TestOrderUpdater_HandleOrderCanceled_UnfreezeFailures(t *testing.T) {
	store := &fakeOrderStore{
		order: &repository.Order{
			OrderID: 1,
			UserID:  10,
			Symbol:  "BTCUSDT",
			Side:    repository.SideBuy,
			Price:   "100000000",
			OrigQty: "200000000",
		},
		cfg: &repository.SymbolConfig{
			BaseAsset:  "BTC",
			QuoteAsset: "USDT",
		},
	}

	failUnfreezer := &failingUnfreezer{resp: &client.UnfreezeResponse{Success: false, ErrorCode: "FAIL"}}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, failUnfreezer, nil, &UpdaterConfig{})

	err := updater.handleOrderCanceled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderCanceledData{
			OrderID:   1,
			UserID:    10,
			LeavesQty: 2 * 1e8,
			Reason:    "USER_CANCELED",
		}),
	})
	if err == nil {
		t.Fatal("expected unfreeze failure error")
	}
}

func TestOrderUpdater_HandleTradeCreated_Errors(t *testing.T) {
	store := &fakeOrderStore{
		cfg: &repository.SymbolConfig{
			QuoteAsset: "USDT",
		},
	}
	trades := &fakeTradeStore{}
	updater := NewOrderUpdater(nil, store, trades, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	if err := updater.handleTradeCreated(context.Background(), &MatchingEvent{
		Type:      "TRADE_CREATED",
		Symbol:    "BTCUSDT",
		Timestamp: 0,
		Data: mustJSON(t, TradeData{
			TradeID:      100,
			MakerOrderID: 1,
			TakerOrderID: 2,
			MakerUserID:  10,
			TakerUserID:  11,
			Price:        100 * 1e8,
			Qty:          2 * 1e8,
			TakerSide:    repository.SideBuy,
		}),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trades.saved == nil || trades.saved.TimestampMs == 0 {
		t.Fatal("expected trade timestamp set")
	}

	errorUpdater := NewOrderUpdater(nil, store, &errorTradeStore{err: errors.New("save failed")}, &fakeUnfreezer{}, nil, &UpdaterConfig{})
	if err := errorUpdater.handleTradeCreated(context.Background(), &MatchingEvent{
		Type:   "TRADE_CREATED",
		Symbol: "BTCUSDT",
		Data: mustJSON(t, TradeData{
			TradeID:      101,
			MakerOrderID: 1,
			TakerOrderID: 2,
			MakerUserID:  10,
			TakerUserID:  11,
			Price:        100 * 1e8,
			Qty:          2 * 1e8,
			TakerSide:    repository.SideBuy,
		}),
	}); err == nil {
		t.Fatal("expected save trade error")
	}

	addErrStore := &addQtyErrorStore{cfg: &repository.SymbolConfig{QuoteAsset: "USDT"}}
	addErrUpdater := NewOrderUpdater(nil, addErrStore, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})
	if err := addErrUpdater.handleTradeCreated(context.Background(), &MatchingEvent{
		Type:   "TRADE_CREATED",
		Symbol: "BTCUSDT",
		Data: mustJSON(t, TradeData{
			TradeID:      102,
			MakerOrderID: 1,
			TakerOrderID: 2,
			MakerUserID:  10,
			TakerUserID:  11,
			Price:        100 * 1e8,
			Qty:          2 * 1e8,
			TakerSide:    repository.SideBuy,
		}),
	}); err == nil {
		t.Fatal("expected add cumulative error")
	}
}
func TestOrderUpdater_StartCanceledContext(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	store := &fakeOrderStore{}
	updater := NewOrderUpdater(redisClient, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "order-updater-group",
		Consumer:    "order-updater-1",
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := updater.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	cancel()
	time.Sleep(10 * time.Millisecond)
}

func TestOrderUpdater_ConsumeLoopCanceled(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	updater := NewOrderUpdater(redisClient, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "order-updater-group",
		Consumer:    "order-updater-1",
	})
	if err := redisClient.XGroupCreateMkStream(context.Background(), "matching:events", "order-updater-group", "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	updater.consumeLoop(ctx)
}

func TestOrderUpdater_ConsumeLoopTimeout(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	updater := NewOrderUpdater(redisClient, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{
		EventStream: "matching:events",
		Group:       "order-updater-group",
		Consumer:    "order-updater-1",
	})
	if err := redisClient.XGroupCreateMkStream(context.Background(), "matching:events", "order-updater-group", "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	updater.consumeLoop(ctx)
}

func TestOrderUpdater_HandleOrderStatuses(t *testing.T) {
	store := &fakeOrderStore{
		order: &repository.Order{
			OrderID:            1,
			CumulativeQuoteQty: "100",
		},
	}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	if err := updater.handleOrderAccepted(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderAcceptedData{OrderID: 1}),
	}); err != nil {
		t.Fatalf("order accepted: %v", err)
	}
	if err := updater.handleOrderPartiallyFilled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderPartiallyFilledData{OrderID: 1, ExecutedQty: 2}),
	}); err != nil {
		t.Fatalf("order partially filled: %v", err)
	}
	if err := updater.handleOrderFilled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderFilledData{OrderID: 1, ExecutedQty: 3}),
	}); err != nil {
		t.Fatalf("order filled: %v", err)
	}

	if len(store.updateCalls) != 3 {
		t.Fatalf("expected 3 update calls, got %d", len(store.updateCalls))
	}
}

func TestOrderUpdater_HandleOrderStatus_UnmarshalError(t *testing.T) {
	store := &fakeOrderStore{
		order: &repository.Order{OrderID: 1},
	}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	if err := updater.handleOrderAccepted(context.Background(), &MatchingEvent{Data: json.RawMessage("{")}); err == nil {
		t.Fatal("expected unmarshal error for accepted")
	}
	if err := updater.handleOrderPartiallyFilled(context.Background(), &MatchingEvent{Data: json.RawMessage("{")}); err == nil {
		t.Fatal("expected unmarshal error for partial")
	}
	if err := updater.handleOrderFilled(context.Background(), &MatchingEvent{Data: json.RawMessage("{")}); err == nil {
		t.Fatal("expected unmarshal error for filled")
	}
}

func TestOrderUpdater_HandleOrderCanceled_AmountZero(t *testing.T) {
	store := &fakeOrderStore{
		order: &repository.Order{
			OrderID: 1,
			UserID:  10,
			Symbol:  "BTCUSDT",
			Side:    repository.SideSell,
			Price:   "100000000",
		},
		cfg: &repository.SymbolConfig{
			BaseAsset:  "BTC",
			QuoteAsset: "USDT",
		},
	}
	unfreezer := &fakeUnfreezer{}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, unfreezer, nil, &UpdaterConfig{})

	if err := updater.handleOrderCanceled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderCanceledData{
			OrderID:   1,
			UserID:    10,
			LeavesQty: 0,
			Reason:    "USER_CANCELED",
		}),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unfreezer.called {
		t.Fatal("expected no unfreeze when amount is zero")
	}
}

func TestOrderUpdater_HandleOrderCanceled_ConfigError(t *testing.T) {
	store := &errorSymbolStore{
		order: &repository.Order{
			OrderID: 1,
			UserID:  10,
			Symbol:  "BTCUSDT",
			Side:    repository.SideBuy,
			Price:   "100000000",
			OrigQty: "200000000",
		},
		err: errors.New("config error"),
	}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	if err := updater.handleOrderCanceled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderCanceledData{
			OrderID:   1,
			UserID:    10,
			LeavesQty: 2 * 1e8,
			Reason:    "USER_CANCELED",
		}),
	}); err == nil {
		t.Fatal("expected config error")
	}
}

func TestOrderUpdater_HandleOrderCanceled_CancelErrorIgnored(t *testing.T) {
	store := &cancelErrStore{
		order: &repository.Order{
			OrderID: 1,
			UserID:  10,
			Symbol:  "BTCUSDT",
			Side:    repository.SideBuy,
			Price:   "100000000",
			OrigQty: "200000000",
		},
		cfg: &repository.SymbolConfig{
			BaseAsset:  "BTC",
			QuoteAsset: "USDT",
		},
	}
	unfreezer := &fakeUnfreezer{}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, unfreezer, nil, &UpdaterConfig{})

	if err := updater.handleOrderCanceled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderCanceledData{
			OrderID:   1,
			UserID:    10,
			LeavesQty: 2 * 1e8,
			Reason:    "USER_CANCELED",
		}),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !unfreezer.called {
		t.Fatal("expected unfreeze called")
	}
}

func TestOrderUpdater_HandleOrderCanceled_OrderError(t *testing.T) {
	store := &orderErrStore{err: errors.New("get order failed")}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	if err := updater.handleOrderCanceled(context.Background(), &MatchingEvent{
		Data: mustJSON(t, OrderCanceledData{
			OrderID:   1,
			UserID:    10,
			LeavesQty: 2 * 1e8,
			Reason:    "USER_CANCELED",
		}),
	}); err == nil {
		t.Fatal("expected get order error")
	}
}

func TestOrderUpdater_HandleTradeCreated_ConfigError(t *testing.T) {
	store := &errorSymbolStore{
		err: errors.New("config error"),
	}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	if err := updater.handleTradeCreated(context.Background(), &MatchingEvent{
		Type:   "TRADE_CREATED",
		Symbol: "BTCUSDT",
		Data: mustJSON(t, TradeData{
			TradeID:      100,
			MakerOrderID: 1,
			TakerOrderID: 2,
			MakerUserID:  10,
			TakerUserID:  11,
			Price:        100 * 1e8,
			Qty:          2 * 1e8,
			TakerSide:    repository.SideBuy,
		}),
	}); err == nil {
		t.Fatal("expected config error")
	}
}

func TestOrderUpdater_GetCumulativeQuoteQty(t *testing.T) {
	store := &fakeOrderStore{
		order: &repository.Order{OrderID: 1, CumulativeQuoteQty: ""},
	}
	updater := NewOrderUpdater(nil, store, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	value, err := updater.getCumulativeQuoteQty(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 0 {
		t.Fatalf("expected 0, got %d", value)
	}

	store.order.CumulativeQuoteQty = "bad"
	if _, err := updater.getCumulativeQuoteQty(context.Background(), 1); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestOrderUpdater_CalculateUnfreeze(t *testing.T) {
	updater := NewOrderUpdater(nil, &fakeOrderStore{}, &fakeTradeStore{}, &fakeUnfreezer{}, nil, &UpdaterConfig{})

	order := &repository.Order{Side: repository.SideSell}
	cfg := &repository.SymbolConfig{BaseAsset: "BTC", QuoteAsset: "USDT"}
	amount, asset, err := updater.calculateUnfreeze(order, cfg, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amount != 5 || asset != "BTC" {
		t.Fatalf("unexpected unfreeze: %d %s", amount, asset)
	}

	buyOrder := &repository.Order{Side: repository.SideBuy, Price: "bad"}
	if _, _, err := updater.calculateUnfreeze(buyOrder, cfg, 5); err == nil {
		t.Fatal("expected price parse error")
	}
}

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
