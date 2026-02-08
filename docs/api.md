# API Reference

OpenExchange provides REST APIs for trading, market data, and account management.

## 📡 Base URLs

| Environment | URL |
|-------------|-----|
| Development | `http://localhost:8080` |
| Production | `https://api.your-domain.com` |

## 🔐 Authentication

### HMAC Signature (Private Endpoints)

All private endpoints require HMAC-SHA256 signature authentication.

**Required Headers:**

```http
X-API-KEY: your-api-key
X-API-TIMESTAMP: 1703232000000
X-API-NONCE: 550e8400-e29b-41d4-a716-446655440000
X-API-SIGNATURE: hmac-sha256-signature
```

**Signature Generation:**

```go
import "github.com/exchange/common/pkg/signature"

signer := signature.NewSigner("your-api-secret")
canonical := fmt.Sprintf("%d\n%s\n%s\n%s\n%s\n%s",
    timestamp,
    nonce,
    method,
    path,
    queryString,
    bodyHash,
)
signature := signer.Sign(canonical)
```

**Signature Payload:**

| Field | Type | Description |
|-------|------|-------------|
| timestamp | int64 | Unix timestamp in milliseconds |
| nonce | string | UUID for request deduplication |
| method | string | HTTP method (GET, POST, etc.) |
| path | string | Request path (e.g., `/v1/orders`) |
| queryString | string | Sorted query parameters |
| bodyHash | string | SHA256 of request body (empty string if no body) |

### JWT Token (Admin Endpoints)

```http
Authorization: Bearer v1.jwt-token
```

## 📋 Endpoints

### Market Data (Public)

#### Get Order Book

```http
GET /v1/depth?symbol=BTC_USDT&limit=100
```

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Trading pair (e.g., `BTC_USDT`) |
| limit | int | No | Depth limit (default: 100, max: 1000) |

**Response:**

```json
{
  "code": 0,
  "data": {
    "symbol": "BTC_USDT",
    "bids": [
      {"price": "50000.00", "qty": "1.5"},
      {"price": "49999.50", "qty": "0.5"}
    ],
    "asks": [
      {"price": "50001.00", "qty": "2.0"},
      {"price":00", "qty "50002.": "1.0"}
    ],
    "timestamp": 1703232000000
  }
}
```

#### Get Recent Trades

```http
GET /v1/trades?symbol=BTC_USDT&limit=50
```

**Response:**

```json
{
  "code": 0,
  "data": [
    {
      "id": "12345",
      "symbol": "BTC_USDT",
      "price": "50001.00",
      "qty": "0.5",
      "side": "SELL",
      "time": 1703232000000
    }
  ]
}
```

#### Get 24h Ticker

```http
GET /v1/ticker?symbol=BTC_USDT
```

**Response:**

```json
{
  "code": 0,
  "data": {
    "symbol": "BTC_USDT",
    "open": "49500.00",
    "high": "50200.00",
    "low": "49200.00",
    "close": "50001.00",
    "volume": "1234.56",
    "quoteVolume": "61728000.00",
    "priceChange": "501.00",
    "priceChangePercent": "1.01",
    "time": 1703232000000
  }
}
```

### Trading (Private)

#### Create Order

```http
POST /v1/orders
Content-Type: application/json

{
  "symbol": "BTC_USDT",
  "side": "BUY",
  "type": "LIMIT",
  "quantity": "0.5",
  "price": "50000.00",
  "timeInForce": "GTC"
}
```

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| symbol | string | Yes | Trading pair |
| side | string | Yes | `BUY` or `SELL` |
| type | string | Yes | `LIMIT` or `MARKET` |
| quantity | string | Yes | Order quantity |
| price | string | Yes* | Price (*required for LIMIT) |
| timeInForce | string | No | `GTC`, `IOC`, `FOK`, `POST_ONLY` |

**Response:**

```json
{
  "code": 0,
  "data": {
    "orderId": "550e8400-e29b-41d4-a716-446655440000",
    "symbol": "BTC_USDT",
    "status": "NEW",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "0.5",
    "price": "50000.00",
    "filledQty": "0.0",
    "avgPrice": "0.00",
    "createdAt": 1703232000000
  }
}
```

#### Cancel Order

```http
DELETE /v1/orders/{orderId}
```

**Response:**

```json
{
  "code": 0,
  "data": {
    "orderId": "550e8400-e29b-41d4-a716-446655440000",
    "status": "CANCELED"
  }
}
```

#### Get Order

```http
GET /v1/orders/{orderId}
```

**Response:**

```json
{
  "code": 0,
  "data": {
    "orderId": "550e8400-e29b-41d4-a716-446655440000",
    "symbol": "BTC_USDT",
    "status": "FILLED",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "0.5",
    "price": "50000.00",
    "filledQty": "0.5",
    "avgPrice": "50001.00",
    "fee": "0.025",
    "feeCurrency": "USDT",
    "createdAt": 1703232000000,
    "updatedAt": 1703232000500
  }
}
```

#### Get Open Orders

```http
GET /v1/openOrders?symbol=BTC_USDT
```

#### Get Order History

```http
GET /v1/orders?symbol=BTC_USDT&startTime=1703228400000&endTime=1703232000000
```

### Account (Private)

#### Get Balance

```http
GET /v1/balance
```

**Response:**

```json
{
  "code": 0,
  "data": {
    "balances": [
      {
        "asset": "BTC",
        "available": "1.5",
        "frozen": "0.0"
      },
      {
        "asset": "USDT",
        "available": "50000.00",
        "frozen": "500.00"
      }
    ]
  }
}
```

#### Get Ledger

```http
GET /v1/ledger?asset=BTC&limit=50
```

**Response:**

```json
{
  "code": 0,
  "data": [
    {
      "id": "12345",
      "asset": "BTC",
      "amount": "0.5",
      "type": "TRADE",
      "balance": "1.5",
      "txHash": "",
      "orderId": "550e8400-e29b-41d4-a716-446655440000",
      "createdAt": 1703232000000
    }
  ]
}
```

### API Key Management

#### Create API Key

```http
POST /v1/apiKeys
Content-Type: application/json

{
  "permissions": ["TRADE", "READ"],
  "ipWhitelist": ["192.168.1.1"],
  "expireAt": 1734768000000
}
```

**Response:**

```json
{
  "code": 0,
  "data": {
    "apiKey": "550e8400e29b41d4a716446655440000",
    "secret": "xxxxxx", // Only shown once!
    "permissions": ["TRADE", "READ"],
    "expireAt": 1734768000000
  }
}
```

#### List API Keys

```http
GET /v1/apiKeys
```

#### Revoke API Key

```http
DELETE /v1/apiKeys/{apiKeyId}
```

## 🔄 WebSocket API

### Connection

```javascript
// Public WebSocket
const ws = new WebSocket('ws://localhost:8094/ws');

// Private WebSocket
const ws = new WebSocket('ws://localhost:8090/ws/private');
ws.send(JSON.stringify({
    action: 'auth',
    token: 'your-jwt-token'
}));
```

### Subscribe to Channels

```json
{
  "action": "subscribe",
  "channels": ["market.BTC_USDT.depth", "market.BTC_USDT.trades"]
}
```

### Channels

| Channel | Description | Update Type |
|---------|-------------|-------------|
| `market.{symbol}.depth` | Order book | Incremental |
| `market.{symbol}.trades` | Trade executions | Full |
| `market.{symbol}.ticker` | 24h ticker | Full |
| `private.orders` | Order updates | Full |
| `private.trades` | Trade notifications | Full |
| `private.balance` | Balance changes | Full |

### Order Book Depth Update

```json
{
  "channel": "market.BTC_USDT.depth",
  "data": {
    "symbol": "BTC_USDT",
    "bids": [["50000.00", "1.5"], ["49999.50", "0.5"]],
    "asks": [["50001.00", "2.0"], ["50002.00", "1.0"]],
    "timestamp": 1703232000000
  }
}
```

## ⚠️ Error Codes

| Code | Message | Description |
|------|---------|-------------|
| 0 | Success | Request succeeded |
| 1001 | Invalid signature | Authentication failed |
| 1002 | Invalid timestamp | Request expired |
| 1003 | Nonce duplicate | Replay attack detected |
| 2001 | Rate limited | Too many requests |
| 3001 | Invalid symbol | Trading pair not found |
| 3002 | Invalid price | Price out of range |
| 3003 | Invalid quantity | Quantity too small/large |
| 3004 | Insufficient balance | Not enough funds |
| 3005 | Order not found | Order ID invalid |
| 3006 | Order cancelled | Order already cancelled |
| 4001 | Symbol disabled | Trading halted |
| 5001 | System error | Internal error |

## 📊 Rate Limits

| Endpoint Type | Limit |
|---------------|-------|
| Public REST | 100 requests/second |
| Private REST | 50 requests/second |
| WebSocket | 100 messages/second |

## 📖 OpenAPI Specification

Download the OpenAPI specification:

- [Gateway API](../exchange-gateway/api/openapi.yaml)
- [Admin API](../exchange-admin/api/openapi.yaml)
- [Wallet API](../exchange-wallet/api/openapi.yaml)
