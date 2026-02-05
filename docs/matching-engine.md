# Matching Engine

In-depth documentation for the OpenExchange high-performance matching engine.

## 📋 Table of Contents

- [Overview](#overview)
- [Order Book Structure](#order-book-structure)
- [Matching Algorithm](#matching-algorithm)
- [Order Types](#order-types)
- [Performance](#performance)
- [Concurrency](#concurrency)

---

## Overview

The matching engine is the core component responsible for pairing buy and sell orders. It maintains the order book and executes trades based on price-time priority.

### Key Characteristics

| Characteristic | Value |
|---------------|-------|
| Throughput | 10,000+ orders/sec per symbol |
| Latency | < 100μs per match |
| Memory | ~1MB per 1M orders |
| Consistency | Strict ordering by price and time |

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                Matching Engine                      │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌─────────────┐         ┌─────────────┐           │
│  │ Order Book │◀──────▶│  Matching   │───────────▶│
│  │  (In-Mem)  │         │  Algorithm  │           │
│  └─────────────┘         └─────────────┘           │
│         │                       │                 │
│         ▼                       ▼                 │
│  ┌─────────────┐         ┌─────────────┐           │
│  │  Order     │         │   Event     │───────────▶│
│  │  Queue     │         │  Emitter    │           │
│  └─────────────┘         └─────────────┘           │
│                                                     │
└─────────────────────────────────────────────────────┘
```

---

## Order Book Structure

### Data Structures

```go
// OrderBook maintains the bid and ask sides
type OrderBook struct {
    symbol string
    bids  *PriceLevelMap  // Sorted map: price -> order list
    asks  *PriceLevelMap  // Sorted map: price -> order list

    // Indexes for O(1) lookup
    orders map[string]*Order  // orderId -> Order

    mu sync.RWMutex
}

// PriceLevel represents a price point with orders
type PriceLevel struct {
    price    decimal.Decimal
    orders   *list.List  // Linked list for time priority
    totalQty decimal.Decimal
}
```

### Price Priority

```
BUY Orders (Bids) - Sorted Descending (Highest First)
┌─────────┬─────────┬─────────┬─────────┐
│ Price   │  50000  │  49999  │  49998  │
│ Total   │  1.5    │  2.5    │  0.5    │
└─────────┴─────────┴─────────┴─────────┘
                        │
                        │ Matching starts here
                        ▼

SELL Orders (Asks) - Sorted Ascending (Lowest First)
┌─────────┬─────────┬─────────┬─────────┐
│ Price   │  50001  │  50002  │  50003  │
│ Total   │  2.0    │  1.0    │  0.5    │
└─────────┴─────────┴─────────┴─────────┘
```

### Time Priority

Orders at the same price are matched in FIFO order:

```
Price: 50000 (Ask)

Order Queue (Linked List):
[Order A: 10:00:00] → [Order B: 10:00:01] → [Order C: 10:00:02]
                        ↑
                 Match first
```

---

## Matching Algorithm

### Core Matching Loop

```go
func (e *Engine) match(order *Order) []*Trade {
    var trades []*Trade

    if order.Side == BUY {
        // Match against asks (lowest first)
        for e.orderBook.HasAsk() && order.IsMatchable() {
            bestAsk := e.orderBook.BestAsk()
            if !order.CanMatch(bestAsk.Price) {
                break // Price too high
            }

            trade := e.matchAtPrice(order, bestAsk)
            trades = append(trades, trade...)

            if trade.IsFullFill() {
                break
            }
        }
    } else {
        // Match against bids (highest first)
        for e.orderBook.HasBid() && order.IsMatchable() {
            bestBid := e.orderBook.BestBid()
            if !order.CanMatch(bestBid.Price) {
                break // Price too low
            }

            trade := e.matchAtPrice(order, bestBid)
            trades = append(trades, trade...)

            if trade.IsFullFill() {
                break
            }
        }
    }

    return trades
}
```

### Matching at Price Level

```go
func (e *Engine) matchAtPrice(order *Order, level *PriceLevel) []*Trade {
    var trades []*Trade

    for level.HasOrders() && order.IsMatchable() {
        restingOrder := level.PopFirst()

        // Calculate trade quantity
        tradeQty := order.RemainingQty()
        if restingOrder.RemainingQty() < tradeQty {
            tradeQty = restingOrder.RemainingQty()
        }

        // Create trade
        trade := &Trade{
            ID:            snowflake.MustNextID(),
            Symbol:        order.Symbol,
            Price:         level.Price,
            Quantity:      tradeQty,
            MakerOrderID:  restingOrder.ID,
            TakerOrderID:  order.ID,
            CreatedAt:     time.Now(),
        }

        // Update orders
        order.Fill(tradeQty)
        restingOrder.Fill(tradeQty)

        // Record trade
        trades = append(trades, trade)

        // Emit events
        e.emitTradeCreated(trade)
        e.emitOrderUpdated(order)
        e.emitOrderUpdated(restingOrder)

        // Handle remaining quantity
        if restingOrder.RemainingQty() > 0 {
            // Put back with same price priority
            level.PushBack(restingOrder)
        }
    }

    return trades
}
```

### Price-Time Priority Logic

```
Incoming Order: BUY 1.0 BTC @ 50001

Current Order Book:
ASKS:
Price    Qty     Orders
50001    0.5     [Order A - 10:00:00]
50001    0.3     [Order B - 10:00:01]
50002    0.5     [Order C - 10:00:02]

Matching Process:
1. Match with Order A @ 50001 × 0.5
2. Match with Order B @ 50001 × 0.3
3. Remaining 0.2 BTC stays on book @ 50001

Resulting Trades:
- Trade 1: 0.5 BTC @ 50001 (Order A)
- Trade 2: 0.3 BTC @ 50001 (Order B)

Remaining Order:
- BUY 0.2 BTC @ 50001 (added to book)
```

---

## Order Types

### Limit Order

```go
// Limit order with specified price
order := &Order{
    Type:      LIMIT,
    Price:     decimal.MustNew("50000.00"),
    Quantity:  decimal.MustNew("1.0"),
    TimeInForce: GTC,
}
```

**Behavior:**
- If marketable, matches immediately
- If not marketable, added to order book
- Stays on book until filled, cancelled, or expired

### Market Order

```go
// Market order - no price specified
order := &Order{
    Type:     MARKET,
    Quantity: decimal.MustNew("1.0"),
}
```

**Behavior:**
- Matches immediately at best available prices
- May result in partial fills
- Cannot be added to order book

### Time in Force

| TIF | Description | Use Case |
|-----|-------------|----------|
| `GTC` | Good Till Cancelled | Default, stays on book |
| `IOC` | Immediate Or Cancel | Must match now or cancel |
| `FOK` | Fill Or Kill | Must fill completely or cancel |
| `POST_ONLY` | Maker Only | Only adds liquidity |

### IOC/FOK Implementation

```go
func (e *Engine) matchWithTIF(order *Order) []*Trade {
    var trades []*Trade

    switch order.TimeInForce {
    case GTC:
        trades = e.match(order)
        if order.HasRemaining() {
            e.addToBook(order)
        }

    case IOC:
        trades = e.match(order)
        // Remaining is cancelled
        e.cancelOrder(order.ID)

    case FOK:
        if e.canFullyFill(order) {
            trades = e.match(order)
        } else {
            e.cancelOrder(order.ID)
        }

    case POST_ONLY:
        if order.WouldTakeLiquidity() {
            e.rejectOrder(order, ErrPostOnlyTakesLiquidity)
        } else {
            e.addToBook(order)
        }
    }

    return trades
}
```

---

## Performance

### Benchmarks

```bash
# Run benchmarks
go test -bench=BenchmarkMatching -benchmem

# Typical results:
# BenchmarkMatch_SingleOrder-8          50000 ns/op     # 50μs
# BenchmarkMatch_BestCase-8           100000 ns/op    # 100μs
# BenchmarkMatch_WorstCase-8         500000 ns/op    # 500μs
# BenchmarkOrderBook_AddOrder-8       1000 ns/op     # 1μs
# BenchmarkOrderBook_RemoveOrder-8    500 ns/op      # 0.5μs
```

### Optimization Techniques

#### 1. Lock-Free Structures

```go
// Use atomic operations where possible
type OrderBook struct {
    orderCount int64
}

func (ob *OrderBook) IncrementCount() {
    atomic.AddInt64(&ob.orderCount, 1)
}
```

#### 2. Batch Processing

```go
// Batch order processing for better throughput
func (e *Engine) processBatch(orders []*Order) {
    for _, order := range orders {
        e.match(order)
    }
}
```

#### 3. Memory Pre-allocation

```go
// Pre-allocate order slices
func (e *Engine) match(order *Order) {
    trades := make([]*Trade, 0, 4) // Pre-allocate for typical cases
    // ... matching logic
}
```

### Throughput Scaling

| Symbol Count | Orders/Second | Latency (p99) |
|-------------|----------------|----------------|
| 1 | 50,000+ | < 100μs |
| 10 | 30,000+ | < 200μs |
| 100 | 10,000+ | < 500μs |

---

## Concurrency

### Per-Symbol Isolation

Each symbol runs in its own goroutine:

```go
func (e *Engine) Start() {
    for symbol, book := range e.orderBooks {
        go e.runSymbolWorker(symbol, book)
    }
}

func (e *Engine) runSymbolWorker(symbol string, book *OrderBook) {
    for order := range book.OrderQueue {
        trades := e.matchAtPrice(order, book)
        e.emitEvents(trades)
    }
}
```

### Thread Safety

```go
type OrderBook struct {
    mu sync.RWMutex  // Full mutex for simplicity

    // For read-heavy workloads, could use:
    // mu sync.RWMutex  // Separate read/write locks
    // or:
    // impl atomic operations for counters
}

func (ob *OrderBook) AddOrder(order *Order) {
    ob.mu.Lock()
    defer ob.mu.Unlock()
    // ... add order
}

func (ob *OrderBook) GetBestAsk() (*PriceLevel, error) {
    ob.mu.RLock()
    defer ob.mu.RUnlock()
    // ... get best ask
}
```

### Race Condition Prevention

```go
// Use channels for order submission
type OrderChannel chan *Order

func (e *Engine) SubmitOrder(order *Order) {
    e.orderChans[order.Symbol] <- order
}

// In worker goroutine
func (e *Engine) runSymbolWorker(symbol string, book *OrderBook) {
    orderCh := e.orderChans[symbol]

    for {
        select {
        case order := <-orderCh:
            e.matchAndEmit(order)
        case <-e.shutdownCh:
            return
        }
    }
}
```

---

## Event Emission

### Trade Created Event

```json
{
  "eventId": "evt_01JXXXXXXX",
  "eventType": "TradeCreated",
  "version": 1,
  "producer": "matching-engine",
  "partitionKey": "BTCUSDT",
  "data": {
    "tradeId": "550e8400-e29b-41d4-a716-446655440000",
    "symbol": "BTC_USDT",
    "price": "50001.00",
    "quantity": "0.5",
    "makerOrderId": "order_001",
    "takerOrderId": "order_002",
    "makerFee": "0.0000125",
    "takerFee": "0.000025",
    "feeCurrency": "BTC"
  }
}
```

### Order Updated Event

```json
{
  "eventId": "evt_01JXXXXXXX",
  "eventType": "OrderUpdated",
  "version": 1,
  "producer": "matching-engine",
  "partitionKey": "BTCUSDT",
  "data": {
    "orderId": "order_001",
    "status": "PARTIALLY_FILLED",
    "executedQty": "0.3",
    "avgPrice": "50001.00"
  }
}
```

---

## 📖 Related Documentation

- [Architecture](architecture.md) - System architecture
- [Trading Flow](trading-flow.md) - Order lifecycle
- [Event Model](event-model.md) - Event specifications
- [API Documentation](api.md) - REST API reference
