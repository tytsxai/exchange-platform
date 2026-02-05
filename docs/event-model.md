# Event Model & Message Specification

Event-driven communication patterns for OpenExchange microservices.

## 📋 Table of Contents

- [Event Envelope](#1-event-envelope)
- [Topics & Partition Keys](#2-topics--partition-keys)
- [Event Versioning](#3-event-versioning)
- [Core Events](#4-core-events)
- [Market Data WebSocket](#5-market-data-websocket)
- [Idempotency & Retry](#6-idempotency-retry)
- [Private Push Events](#7-private-push-events)

---

## 1. Event Envelope

All events are wrapped in a standardized envelope for tracking, idempotency, and versioning.

### Event Envelope Schema

```json
{
  "eventId": "evt_01JXXXXXXX",
  "eventType": "TradeCreated",
  "version": 1,
  "occurredAtMs": 1730000000000,
  "producer": "matching-engine",
  "partitionKey": "BTCUSDT",
  "traceId": "req_01JXXXXXXX",
  "data": {}
}
```

### Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `eventId` | string | Yes | Globally unique identifier (UUID) for deduplication and auditing |
| `eventType` | string | Yes | Event type identifier (e.g., `TradeCreated`) |
| `version` | int | Yes | Event schema version for compatibility |
| `occurredAtMs` | int64 | Yes | Event timestamp in milliseconds (UTC) |
| `producer` | string | Yes | Source service (e.g., `matching-engine`, `clearing-service`) |
| `partitionKey` | string | Yes | Partition key for ordering and scaling |
| `traceId` | string | No | Distributed trace ID for request tracking |
| `data` | object | Yes | Event-specific payload |

### Example Events

**TradeCreated Event:**

```json
{
  "eventId": "evt_01JABC123",
  "eventType": "TradeCreated",
  "version": 1,
  "occurredAtMs": 1730000000000,
  "producer": "matching-engine",
  "partitionKey": "BTCUSDT",
  "traceId": "req_01JXYZ789",
  "data": {
    "tradeId": "550e8400-e29b-41d4-a716-446655440000",
    "symbol": "BTC_USDT",
    "price": "50001.00",
    "quantity": "0.5",
    "quoteQuantity": "25000.50",
    "makerOrderId": "order_001",
    "takerOrderId": "order_002",
    "makerUserId": "user_001",
    "takerUserId": "user_002",
    "makerFee": "0.0000125",
    "takerFee": "0.000025",
    "feeCurrency": "BTC",
    "createdAt": 1730000000000
  }
}
```

**OrderAccepted Event:**

```json
{
  "eventId": "evt_01JDEF456",
  "eventType": "OrderAccepted",
  "version": 1,
  "occurredAtMs": 1730000000500,
  "producer": "order-service",
  "partitionKey": "BTCUSDT",
  "traceId": "req_01JXYZ790",
  "data": {
    "orderId": "order_003",
    "userId": "user_003",
    "symbol": "BTC_USDT",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "1.0",
    "price": "49999.00",
    "timeInForce": "GTC",
    "status": "NEW",
    "createdAt": 1730000000500
  }
}
```

---

## 2. Topics & Partition Keys

### Partition Key Strategy

Events are partitioned by key to ensure ordering within domains:

| Domain | Partition Key | Rationale |
|--------|---------------|-----------|
| Trading/Matching | `symbol` | Orders and trades must be ordered per trading pair |
| Funds/Ledger | `userId` | Balance updates must be ordered per user |
| Wallet | `userId` | Deposits/withdrawals ordered per user |

### Topic Structure

```
exchange:orders       # Order submission stream (key: symbol)
exchange:events       # Core events stream (key: varies)
exchange:trades       # Trade events (key: symbol)
exchange:ledger       # Ledger events (key: userId)
exchange:wallet       # Wallet events (key: userId)
private:{userId}:events  # Private user events (Pub/Sub)
```

### Environment Variables

```bash
# Stream configuration
ORDER_STREAM=exchange:orders
EVENT_STREAM=exchange:events
PRIVATE_USER_EVENT_CHANNEL=private:user:{userId}:events
```

### Service Mapping

| Service | Produces | Consumes |
|---------|----------|----------|
| `exchange-order` | `exchange:orders` | - |
| `exchange-matching` | `exchange:events` | `exchange:orders` |
| `exchange-clearing` | `exchange:ledger` | `exchange:events` |
| `exchange-marketdata` | - | `exchange:events` |
| `exchange-gateway` | `private:{userId}:events` | `exchange:ledger` |

---

## 3. Event Versioning

### Versioning Rules

1. **Backward-Compatible Changes**: Adding new fields is allowed
2. **Breaking Changes**: Increment version number, support both for migration
3. **Field Documentation**: Every field must specify:
   - Whether optional
   - Default value
   - Unit/precision

### Version Migration Strategy

```go
// Handle multiple versions
func ProcessEvent(envelope EventEnvelope) error {
    switch envelope.Version {
    case 1:
        return processV1(envelope)
    case 2:
        return processV2(envelope)
    default:
        return fmt.Errorf("unsupported event version: %d", envelope.Version)
    }
}
```

---

## 4. Core Events

### Order Events

| Event Type | Description | Producer |
|------------|-------------|----------|
| `OrderAccepted` | Order passed validation and queued | Order Service |
| `OrderRejected` | Order rejected with reason | Order Service |
| `OrderCanceled` | Order successfully canceled | Matching Engine |
| `OrderUpdated` | Order status update (partial fill) | Matching Engine |

#### Order State Machine

```
INIT → NEW → PARTIALLY_FILLED → FILLED
              ↓
         CANCELED
              ↓
         EXPIRED (IOC/FOK)
```

#### Order Event Fields

```json
{
  "orderId": "string",
  "clientOrderId": "string",
  "userId": "string",
  "symbol": "string",
  "side": "BUY|SELL",
  "type": "LIMIT|MARKET",
  "quantity": "decimal",
  "price": "decimal",
  "timeInForce": "GTC|IOC|FOK|POST_ONLY",
  "status": "INIT|NEW|PARTIALLY_FILLED|FILLED|CANCELED|REJECTED|EXPIRED",
  "executedQty": "decimal",
  "avgPrice": "decimal",
  "reason": "string"
}
```

### Trade Events

| Event Type | Description | Producer |
|------------|-------------|----------|
| `TradeCreated` | Matched trade execution | Matching Engine |

#### Trade Event Fields

```json
{
  "tradeId": "string",
  "symbol": "string",
  "price": "decimal",
  "quantity": "decimal",
  "quoteQuantity": "decimal",
  "makerOrderId": "string",
  "takerOrderId": "string",
  "makerUserId": "string",
  "takerUserId": "string",
  "makerFee": "decimal",
  "takerFee": "decimal",
  "feeCurrency": "string",
  "timestamp": "int64"
}
```

### Ledger Events

| Event Type | Description | Producer |
|------------|-------------|----------|
| `LedgerEntryCreated` | New ledger entry | Clearing Service |
| `BalanceChanged` | Balance update notification | Clearing Service |

#### Ledger Event Fields

```json
{
  "ledgerId": "string",
  "userId": "string",
  "asset": "string",
  "amount": "decimal",
  "availableAfter": "decimal",
  "frozenAfter": "decimal",
  "reason": "TRADE|DEPOSIT|WITHDRAWAL|FEE|FREEZE|UNFREEZE",
  "refType": "ORDER|TRADE|DEPOSIT|WITHDRAWAL",
  "refId": "string",
  "idempotencyKey": "string"
}
```

### Wallet Events

| Event Type | Description |
|------------|-------------|
| `DepositDetected` | Chain deposit detected (unconfirmed) |
| `DepositConfirmed` | Required confirmations reached |
| `DepositCredited` | Deposit credited to account |
| `WithdrawRequested` | Withdrawal request created |
| `WithdrawApproved` | Withdrawal approved |
| `WithdrawRejected` | Withdrawal rejected |
| `WithdrawSent` | Transaction broadcasted |
| `WithdrawCompleted` | Withdrawal completed |

### Admin Events

| Event Type | Description |
|------------|-------------|
| `SymbolConfigUpdated` | Trading pair configuration changed |
| `FeeConfigUpdated` | Fee structure changed |
| `KillSwitchChanged` | Emergency trading halt |
| `RiskRuleUpdated` | Risk control rule update |

---

## 5. Market Data WebSocket

### WebSocket Envelope

```json
{
  "channel": "market.BTC_USDT.book",
  "seq": 12345,
  "timestampMs": 1730000000000,
  "data": {}
}
```

### Channel Types

| Channel | Description | Update Type |
|---------|-------------|------------|
| `market.{symbol}.depth` | Order book | Snapshot/Delta |
| `market.{symbol}.trades` | Trade history | Full |
| `market.{symbol}.ticker` | 24h statistics | Full |
| `market.{symbol}.kline.{interval}` | K-line/Candlestick | Incremental |

### Depth Channel

**Book Snapshot:**

```json
{
  "channel": "market.BTC_USDT.depth",
  "seq": 1000,
  "timestampMs": 1730000000000,
  "data": {
    "symbol": "BTC_USDT",
    "bids": [["50000.00", "1.5"], ["49999.50", "0.5"]],
    "asks": [["50001.00", "2.0"], ["50002.00", "1.0"]],
    "snapshotSeq": 1000
  }
}
```

**Book Delta:**

```json
{
  "channel": "market.BTC_USDT.depth",
  "seq": 1001,
  "prevSeq": 1000,
  "timestampMs": 1730000000100,
  "data": {
    "symbol": "BTC_USDT",
    "bids": [["50000.00", "2.0"]],
    "asks": []
  }
}
```

### Client Recovery Protocol

```javascript
// If prevSeq != lastProcessedSeq, gap detected
if (data.prevSeq !== lastSeq) {
    // Request full snapshot
    await resubscribeToChannel(channel, { snapshot: true });
}
```

---

## 6. Idempotency, Retry & Replay

### Idempotency Keys

| Domain | Idempotency Key Pattern |
|---------|------------------------|
| Ledger | `ledger:{reason}:{refType}:{refId}:{userId}:{asset}` |
| Withdrawal | `withdraw:{userId}:{asset}:{clientRequestId}` |
| Events | `eventId` (globally unique) |

### Implementation

```go
// Database constraint for idempotency
CREATE TABLE ledger_entries (
    id BIGINT PRIMARY KEY,
    idempotency_key VARCHAR(256) UNIQUE NOT NULL,
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    amount DECIMAL(32, 18) NOT NULL,
    -- ... other fields
);

// Deduplication in consumer
func (s *LedgerConsumer) Process(ctx context.Context, event *LedgerEvent) error {
    // Check if already processed
    exists, err := s.repo.IdempotencyKeyExists(ctx, event.IdempotencyKey)
    if err != nil {
        return err
    }
    if exists {
        return nil // Already processed, skip
    }

    // Process event
    _, err = s.repo.CreateLedgerEntry(ctx, event.LedgerEntry)
    if err != nil {
        return err
    }

    // Record idempotency key
    return s.repo.RecordIdempotencyKey(ctx, event.IdempotencyKey)
}
```

### Retry Policy

- **At-Least-Once Delivery**: Events may be redelivered
- **Idempotent Consumers**: Must handle duplicate events safely
- **Backoff Strategy**: Exponential backoff with jitter

### Replay Capability

- Trade events can be replayed for:
  - Market data reconstruction
  - Report regeneration
  - Consistency verification

---

## 7. Private Push Events

### Private WebSocket Endpoint

```
ws://gateway:8090/ws/private
```

### Authentication

```
ws://gateway:8090/ws/private?apiKey=<apiKey>&timestamp=<ts>&nonce=<uuid>&signature=<sig>
```

### Private Event Types

| Event Type | Description |
|------------|-------------|
| `order.update` | Order status changes |
| `trade.created` | User's trade executions |
| `balance.update` | Balance changes |

### Private Event Envelope

```json
{
  "type": "order.update",
  "userId": "123456",
  "timestampMs": 1730000000000,
  "data": {
    "orderId": "order_001",
    "status": "FILLED",
    "executedQty": "0.5",
    "avgPrice": "50001.00"
  }
}
```

### Redis Pub/Sub Channel

```
private:user:{userId}:events
```

### Connection Health

- Gateway's `/ready` endpoint monitors `privateEventsConsumer` health
- If Redis Pub/Sub consumer fails, `/ready` returns `503`

---

## 📖 Related Documentation

- [Architecture](architecture.md) - System architecture
- [API Documentation](api.md) - REST and WebSocket APIs
- [Trading Flow](trading-flow.md) - Order lifecycle
- [Data Models](data-models.md) - Database schemas
