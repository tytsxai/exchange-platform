package service

import (
	"testing"
)

func TestDepthStruct(t *testing.T) {
	depth := &Depth{
		Symbol:       "BTCUSDT",
		Bids:         []PriceLevel{{Price: 50000, Qty: 100}},
		Asks:         []PriceLevel{{Price: 51000, Qty: 200}},
		LastUpdateID: 1,
		TimestampMs:  1000000,
	}

	if depth.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", depth.Symbol)
	}
	if len(depth.Bids) != 1 {
		t.Fatalf("expected 1 bid, got %d", len(depth.Bids))
	}
	if len(depth.Asks) != 1 {
		t.Fatalf("expected 1 ask, got %d", len(depth.Asks))
	}
}

func TestPriceLevelStruct(t *testing.T) {
	level := PriceLevel{
		Price: 50000,
		Qty:   100,
	}

	if level.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", level.Price)
	}
	if level.Qty != 100 {
		t.Fatalf("expected Qty=100, got %d", level.Qty)
	}
}

func TestTradeStruct(t *testing.T) {
	trade := &Trade{
		TradeID:     1,
		Symbol:      "BTCUSDT",
		Price:       50000,
		Qty:         100,
		TakerSide:   1,
		TimestampMs: 1000000,
	}

	if trade.TradeID != 1 {
		t.Fatalf("expected TradeID=1, got %d", trade.TradeID)
	}
	if trade.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", trade.Symbol)
	}
	if trade.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", trade.Price)
	}
}

func TestTickerStruct(t *testing.T) {
	ticker := &Ticker{
		Symbol:             "BTCUSDT",
		LastPrice:          50000,
		PriceChange:        1000,
		PriceChangePercent: "+2.00%",
		HighPrice:          51000,
		LowPrice:           49000,
		Volume:             10000,
		QuoteVolume:        500000000,
		OpenPrice:          49000,
		TradeCount:         1000,
		OpenTimeMs:         1000000,
		CloseTimeMs:        2000000,
	}

	if ticker.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", ticker.Symbol)
	}
	if ticker.LastPrice != 50000 {
		t.Fatalf("expected LastPrice=50000, got %d", ticker.LastPrice)
	}
	if ticker.HighPrice != 51000 {
		t.Fatalf("expected HighPrice=51000, got %d", ticker.HighPrice)
	}
	if ticker.LowPrice != 49000 {
		t.Fatalf("expected LowPrice=49000, got %d", ticker.LowPrice)
	}
}

func TestEventStruct(t *testing.T) {
	event := &Event{
		Channel:     "market.BTCUSDT.trades",
		Seq:         1,
		TimestampMs: 1000000,
		Data:        nil,
	}

	if event.Channel != "market.BTCUSDT.trades" {
		t.Fatalf("expected Channel=market.BTCUSDT.trades, got %s", event.Channel)
	}
	if event.Seq != 1 {
		t.Fatalf("expected Seq=1, got %d", event.Seq)
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	if cfg.EventStream != "matching:events" {
		t.Fatalf("expected EventStream=matching:events, got %s", cfg.EventStream)
	}
	if cfg.Group != "marketdata" {
		t.Fatalf("expected Group=marketdata, got %s", cfg.Group)
	}
	if cfg.Consumer != "consumer-1" {
		t.Fatalf("expected Consumer=consumer-1, got %s", cfg.Consumer)
	}
}

func TestMatchingEventStruct(t *testing.T) {
	event := MatchingEvent{
		Type:      "TRADE_CREATED",
		Symbol:    "BTCUSDT",
		Seq:       1,
		Timestamp: 1000000,
	}

	if event.Type != "TRADE_CREATED" {
		t.Fatalf("expected Type=TRADE_CREATED, got %s", event.Type)
	}
	if event.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", event.Symbol)
	}
}

func TestTradeDataStruct(t *testing.T) {
	data := TradeData{
		TradeID:      1,
		MakerOrderID: 100,
		TakerOrderID: 200,
		MakerUserID:  10,
		TakerUserID:  20,
		Price:        50000,
		Qty:          100,
		TakerSide:    1,
	}

	if data.TradeID != 1 {
		t.Fatalf("expected TradeID=1, got %d", data.TradeID)
	}
	if data.Price != 50000 {
		t.Fatalf("expected Price=50000, got %d", data.Price)
	}
}

func TestOrderAcceptedDataStruct(t *testing.T) {
	data := OrderAcceptedData{
		OrderID: 1,
		UserID:  100,
		Side:    1,
		Price:   50000,
		Qty:     100,
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.Side != 1 {
		t.Fatalf("expected Side=1, got %d", data.Side)
	}
}

func TestOrderCanceledDataStruct(t *testing.T) {
	data := OrderCanceledData{
		OrderID:   1,
		UserID:    100,
		LeavesQty: 50,
	}

	if data.OrderID != 1 {
		t.Fatalf("expected OrderID=1, got %d", data.OrderID)
	}
	if data.LeavesQty != 50 {
		t.Fatalf("expected LeavesQty=50, got %d", data.LeavesQty)
	}
}

func TestNewMarketDataService(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestMarketDataServiceGetDepthEmpty(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	depth := svc.GetDepth("BTCUSDT", 10)

	if depth == nil {
		t.Fatal("expected non-nil depth")
	}
	if depth.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", depth.Symbol)
	}
	if len(depth.Bids) != 0 {
		t.Fatalf("expected empty bids, got %d", len(depth.Bids))
	}
	if len(depth.Asks) != 0 {
		t.Fatalf("expected empty asks, got %d", len(depth.Asks))
	}
}

func TestMarketDataServiceGetTradesEmpty(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	trades := svc.GetTrades("BTCUSDT", 10)

	if trades == nil {
		t.Fatal("expected non-nil trades")
	}
	if len(trades) != 0 {
		t.Fatalf("expected empty trades, got %d", len(trades))
	}
}

func TestMarketDataServiceGetTickerEmpty(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	ticker := svc.GetTicker("BTCUSDT")

	if ticker == nil {
		t.Fatal("expected non-nil ticker")
	}
	if ticker.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", ticker.Symbol)
	}
}

func TestMarketDataServiceGetAllTickersEmpty(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	tickers := svc.GetAllTickers()

	if tickers == nil {
		t.Fatal("expected non-nil tickers")
	}
	if len(tickers) != 0 {
		t.Fatalf("expected empty tickers, got %d", len(tickers))
	}
}

func TestMarketDataServiceSubscribe(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	ch := svc.Subscribe("market.BTCUSDT.trades")

	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}

func TestMarketDataServiceUnsubscribe(t *testing.T) {
	cfg := &Config{
		EventStream: "matching:events",
		Group:       "marketdata",
		Consumer:    "consumer-1",
	}

	svc := NewMarketDataService(nil, cfg)
	ch := svc.Subscribe("market.BTCUSDT.trades")
	svc.Unsubscribe("market.BTCUSDT.trades", ch)
	// Should not panic
}

func TestInsertLevel(t *testing.T) {
	// Test descending (bids)
	levels := []PriceLevel{
		{Price: 50000, Qty: 100},
		{Price: 49000, Qty: 200},
	}

	levels = insertLevel(levels, PriceLevel{Price: 49500, Qty: 150}, true)
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels[1].Price != 49500 {
		t.Fatalf("expected price=49500 at index 1, got %d", levels[1].Price)
	}

	// Test ascending (asks)
	levels = []PriceLevel{
		{Price: 50000, Qty: 100},
		{Price: 51000, Qty: 200},
	}

	levels = insertLevel(levels, PriceLevel{Price: 50500, Qty: 150}, false)
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels[1].Price != 50500 {
		t.Fatalf("expected price=50500 at index 1, got %d", levels[1].Price)
	}
}

func TestInsertLevelUpdate(t *testing.T) {
	levels := []PriceLevel{
		{Price: 50000, Qty: 100},
		{Price: 49000, Qty: 200},
	}

	// Update existing level
	levels = insertLevel(levels, PriceLevel{Price: 50000, Qty: 300}, true)
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(levels))
	}
	if levels[0].Qty != 300 {
		t.Fatalf("expected qty=300, got %d", levels[0].Qty)
	}
}

func TestInsertLevelDelete(t *testing.T) {
	levels := []PriceLevel{
		{Price: 50000, Qty: 100},
		{Price: 49000, Qty: 200},
	}

	// Delete level (qty=0)
	levels = insertLevel(levels, PriceLevel{Price: 50000, Qty: 0}, true)
	if len(levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(levels))
	}
	if levels[0].Price != 49000 {
		t.Fatalf("expected price=49000, got %d", levels[0].Price)
	}
}

func TestFormatPercent(t *testing.T) {
	// Positive
	result := formatPercent(2.5)
	if result[0] != '+' {
		t.Fatal("expected positive percent to start with +")
	}

	// Negative
	result = formatPercent(-2.5)
	if result[0] == '+' {
		t.Fatal("expected negative percent not to start with +")
	}
}

func TestMatchingEventTypes(t *testing.T) {
	types := []string{
		"TRADE_CREATED",
		"ORDER_ACCEPTED",
		"ORDER_CANCELED",
		"ORDER_FILLED",
	}

	for _, typ := range types {
		event := MatchingEvent{Type: typ}
		if event.Type != typ {
			t.Fatalf("expected Type=%s, got %s", typ, event.Type)
		}
	}
}

func TestTakerSideValues(t *testing.T) {
	// 1 = BUY, 2 = SELL
	trade := &Trade{TakerSide: 1}
	if trade.TakerSide != 1 {
		t.Fatalf("expected TakerSide=1 (BUY), got %d", trade.TakerSide)
	}

	trade.TakerSide = 2
	if trade.TakerSide != 2 {
		t.Fatalf("expected TakerSide=2 (SELL), got %d", trade.TakerSide)
	}
}
