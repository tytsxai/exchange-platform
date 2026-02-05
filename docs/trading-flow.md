# Trading Flow

Complete guide to the order lifecycle and trading flow in OpenExchange.

## 📊 Order Lifecycle

```
┌─────────┐     ┌──────────┐     ┌─────────────┐     ┌─────────┐     ┌──────────┐
│  INIT   │ ──▶ │   NEW    │ ──▶ │ PARTIAL_FILL│ ──▶ │ FILLED  │ ──▶ │ COMPLETE │
└─────────┘     └──────────┘     └─────────────┘     └─────────┘     └──────────┘
                           │              │
                           ▼              ▼
                     ┌──────────┐   ┌──────────┐
                     │ CANCELED │   │  EXPIRED │
                     └──────────┘   └──────────┘
```

## 🔄 Detailed Flow

### 1. Order Creation

```
Client ──▶ Gateway ──▶ Order Service ──▶ Clearing (Check Balance) ──▶ Matching Engine
                │                                  │
                │                                  ▼
                │                            Freeze Funds
                │
                ▼
          Return Order ID
```

**Steps:**

1. Client sends order request to Gateway
2. Gateway validates signature and rate limits
3. Gateway forwards to Order Service
4. Order Service validates:
   - Symbol exists and is enabled
   - Price and quantity within bounds
   - User has sufficient balance
5. Order Service freezes required funds via Clearing
6. Order Service persists order to database
7. Order Service sends order to Matching Engine via Redis Stream
8. Gateway returns order ID to client

**Request:**

```json
{
  "symbol": "BTC_USDT",
  "side": "BUY",
  "type": "LIMIT",
  "quantity": "0.5",
  "price": "50000.00",
  "timeInForce": "GTC"
}
```

**Response:**

```json
{
  "orderId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "NEW",
  "symbol": "BTC_USDT",
  "quantity": "0.5",
  "price": "50000.00"
}
```

### 2. Order Matching

```
┌─────────────────────────────────────────────────────────────┐
│                    Matching Engine                          │
├─────────────────────────────────────────────────────────────┤
│  Order Book: BTC_USDT                                      │
│                                                             │
│  BIDS (Buyers)              ASKS (Sellers)                 │
│  Price    Qty               Price    Qty                    │
│  49999.50  0.5             50001.00  1.0                   │
│  49999.00  1.0             50002.00  2.0                   │
│  49998.00  2.5             50003.00  0.5                   │
└─────────────────────────────────────────────────────────────┘
                    │
                    ▼
         Incoming BUY Order: 0.5 @ 50001.00
                    │
                    ▼
         Match with ASK: 50001.00 × 0.5
                    │
                    ▼
         Create Trade, Update Order Book
```

**Matching Algorithm:**

1. Order placed in order book (if limit) or matches immediately (if market)
2. Price-time priority: Best price first, then earliest time
3. Orders matched in continuous fashion
4. Each match creates a trade
5. Trade events emitted to Redis Streams

### 3. Trade Settlement

```
Matching Engine ──▶ Clearing Service ──▶ Database
                      │
         ┌────────────┼────────────┐
         ▼            ▼            ▼
    Update Buyer   Update Seller   Record Trade
    Balance       Balance
```

**Clearing Process:**

1. Receive trade event from Matching Engine
2. Calculate fees (maker/taker)
3. Update buyer balance:
   - Debit frozen funds
   - Credit purchased asset
   - Deduct fee
4. Update seller balance:
   - Debit sold asset
   - Credit frozen quote currency
   - Deduct fee
5. Create ledger entries for audit trail
6. Emit balance events

### 4. Order Status Updates

```
Matching Engine ──▶ Redis Pub/Sub ──▶ Gateway ──▶ Client (WebSocket)
        │
        ▼
   Order Service (persist status)
```

**Status Flow:**

| Status | Description | Trigger |
|--------|-------------|---------|
| `INIT` | Order created | Client request received |
| `NEW` | Order submitted to matching | Validation passed |
| `PARTIALLY_FILLED` | Partial execution | First trade matched |
| `FILLED` | Fully executed | All quantity filled |
| `CANCELED` | User requested cancel | Cancel API call |
| `REJECTED` | System rejected | Validation failure |
| `EXPIRED` | Time limit reached | TIF timeout (IOC/FOK) |

### 5. Order Cancellation

```
Client ──▶ Gateway ──▶ Order Service ──▶ Matching Engine
                                           │
                                           ▼
                                    Remove from Order Book
                                    (if not yet matched)
                                           │
                                           ▼
                                    Update Order Status
                                    Release Frozen Funds
```

## 💰 Fee Calculation

### Maker Fee

Orders that provide liquidity (limit orders that don't immediately match):

```
Maker Fee = Trade Amount × Maker Fee Rate
```

### Taker Fee

Orders that take liquidity (market orders or limit orders that match immediately):

```
Taker Fee = Trade Amount × Taker Fee Rate
```

### Fee Rates

| Tier | 30d Volume (USD) | Maker | Taker |
|------|------------------|-------|-------|
| VIP 0 | < $10,000 | 0.02% | 0.05% |
| VIP 1 | $10,000 - $100,000 | 0.015% | 0.045% |
| VIP 2 | $100,000 - $1,000,000 | 0.01% | 0.04% |
| VIP 3 | > $1,000,000 | 0.005% | 0.035% |

## 🔄 Time in Force

### GTC (Good Till Cancelled)

Order remains active until filled or explicitly cancelled.

```go
// Behavior
if order.timeInForce == GTC {
    // Add to order book
    orderBook.Add(order)
    // Stay until filled or cancelled
}
```

### IOC (Immediate Or Cancel)

Order must match immediately; unfilled portion is cancelled.

```go
// Behavior
if order.timeInForce == IOC {
    // Attempt to match
    matches := orderBook.Match(order)
    // Cancel remaining
    order.Status = CANCELED
    // Release frozen funds
}
```

### FOK (Fill Or Kill)

Order must be completely filled immediately or cancelled.

```go
// Behavior
if order.timeInForce == FOK {
    // Check if can be fully filled
    if canFullyFill(order) {
        matches := orderBook.Match(order)
        // Must match all quantity
        if totalFilled == order.Quantity {
            order.Status = FILLED
        } else {
            order.Status = CANCELED
        }
    } else {
        order.Status = CANCELED
    }
}
```

### POST_ONLY

Order only adds liquidity; if it would take, it's cancelled.

```go
// Behavior
if order.timeInForce == POST_ONLY {
    bestPrice := orderBook.GetBestPrice(order.Side)
    // For BUY: price must be < best ask
    // For SELL: price must be > best bid
    if wouldTakeLiquidity(order) {
        order.Status = REJECTED
    } else {
        orderBook.Add(order)
    }
}
```

## 📊 Order Types

### Limit Order

指定价格成交，买方出价不得高于指定价格，卖方要价不得低于指定价格。

```
BUY LIMIT @ 50000.00
- Will only fill if price <= 50000.00
- Added to order book until filled
```

### Market Order

以当前最优价格立即成交。

```
BUY MARKET
- Fills immediately at best available prices
- Final price may differ from last seen
- Only specify quantity, not price
```

## 🔄 Example Trade Flow

### Scenario: User buys 0.5 BTC at 50000 USDT

**Step 1: Create Order**

```
POST /v1/orders
{
  "symbol": "BTC_USDT",
  "side": "BUY",
  "type": "LIMIT",
  "quantity": "0.5",
  "price": "50000.00"
}

Response:
{
  "orderId": "12345",
  "status": "NEW"
}
```

**Step 2: Matching**

- Order enters order book
- Matches with existing ASK at 50000.00
- Trade created

**Step 3: Settlement**

- Buyer: -25000 USDT (frozen), +0.5 BTC
- Seller: +25000 USDT, -0.5 BTC
- Fees deducted

**Step 4: Notifications**

- WebSocket message to both users
- Balance updates
- Order status updates to FILLED

## ⚠️ Error Handling

### Common Errors

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `INSUFFICIENT_BALANCE` | Not enough funds | Deposit more |
| `INVALID_PRICE` | Price out of range | Adjust price |
| `INVALID_QUANTITY` | Quantity too small/large | Adjust quantity |
| `SYMBOL_DISABLED` | Trading pair halted | Wait or cancel |
| `ORDER_NOT_FOUND` | Order ID invalid | Verify ID |
| `RATE_LIMITED` | Too many requests | Slow down |

### Retry Logic

```
Client ──▶ Request ──▶ 429 (Rate Limited)
                      │
                      ▼
               Wait 1 second
                      │
                      ▼
               Retry Request
                      │
              3 retries max
                      │
                      ▼
              Return error
```

## 📈 Performance Characteristics

| Operation | Latency |
|-----------|---------|
| Order Creation | < 10ms |
| Order Matching | < 100μs |
| Trade Settlement | < 5ms |
| WebSocket Notification | < 50ms |
| Order Book Update | < 10ms |

## 🔗 Related Documentation

- [API Documentation](api.md) - REST API reference
- [Architecture](architecture.md) - System design
- [Data Models](data-models.md) - Database schemas
