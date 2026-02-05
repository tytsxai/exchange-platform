# Architecture Overview

OpenExchange is a high-performance cryptocurrency exchange built with a microservices architecture using Go, PostgreSQL, and Redis Streams.

## 🏗️ System Architecture

### High-Level Design

```
                                    ┌─────────────────────────────────────┐
                                    │          Public Internet            │
                                    └─────────────────────────────────────┘
                                                   │
                              ┌────────────────────┼────────────────────┐
                              │                    │                    │
                              ▼                    ▼                    ▼
                     ┌─────────────┐      ┌─────────────┐      ┌─────────────┐
                     │  REST API   │      │  WebSocket  │      │  Metrics    │
                     │  (8080)     │      │  (8094)     │      │  (8080)     │
                     └─────────────┘      └─────────────┘      └─────────────┘
                              │                    │
                              │                    │
                              ▼                    │
                     ┌─────────────┐               │
                     │  Gateway    │◀──────────────┘
                     │  Service    │
                     │  (8080)     │
                     └──────┬──────┘
                            │
                            │ gRPC
                            ▼
              ┌─────────────────────────────┐
              │      Service Mesh           │
              │   (Internal Communication)  │
              └─────────────────────────────┘
                            │
          ┌─────────────────┼─────────────────┐
          │                 │                 │
          ▼                 ▼                 ▼
   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
   │   Order     │   │  Matching   │   │   User      │
   │  Service    │   │  Engine     │   │  Service    │
   │  (8081)     │   │  (8082)     │   │  (8085)     │
   └──────┬──────┘   └──────┬──────┘   └─────────────┘
          │                 │
          │                 │ Events (Redis Streams)
          │                 ▼
          │         ┌─────────────┐
          │         │  Clearing   │
          │         │  Service    │
          │         │  (8083)     │
          │         └──────┬──────┘
          │                │
          │                ▼
          │    ┌──────────────────────┐
          │    │  Event Distribution  │
          │    │  (Redis Pub/Sub)     │
          │    └──────────────────────┘
          │                │
          │      ┌─────────┴─────────┐
          │      │                   │
          ▼      ▼                   ▼
   ┌─────────────┐         ┌─────────────┐    ┌─────────────┐
   │Market Data  │         │   Wallet    │    │   Admin     │
   │  Service    │         │  Service    │    │  Service    │
   │  (8084)     │         │  (8086)     │    │  (8087)     │
   └─────────────┘         └─────────────┘    └─────────────┘
          │
          ▼
   ┌─────────────────────────────────────┐
   │         Data Layer                   │
   │  ┌─────────┐  ┌─────────┐  ┌───────┐ │
   │  │PostgreSQL│  │  Redis  │  │ Jaeger│ │
   │  │  5436   │  │  6380   │  │ 16686 │ │
   │  └─────────┘  └─────────┘  └───────┘ │
   └─────────────────────────────────────┘
```

## 🔧 Core Components

### 1. Gateway Service (`exchange-gateway`)

The entry point for all client requests.

**Responsibilities:**
- REST API routing and proxying
- HMAC-SHA256 signature verification
- Rate limiting (IP-based and user-based)
- Request validation and sanitization
- CORS handling
- WebSocket upgrade handling

**Key Features:**
- Non-blocking request handling
- Configurable rate limits
- Request deduplication (nonce-based)
- Comprehensive logging and tracing

### 2. User Service (`exchange-user`)

Manages user accounts and authentication.

**Responsibilities:**
- User registration and login
- API key generation and management
- JWT token issuance
- Password hashing (bcrypt)
- User profile management

**Data Model:**
- Users table (id, email, status, created_at)
- API keys table (key_hash, secret_encrypted, permissions)
- Login history

### 3. Order Service (`exchange-order`)

Handles order lifecycle management.

**Responsibilities:**
- Order creation and validation
- Order cancellation
- Order query (open, filled, history)
- Order state management
- Symbol configuration

**Order Flow:**
```
1. Receive order request
2. Validate (balance, limits, symbol)
3. Generate order ID (snowflake)
4. Persist to database
5. Send to matching engine via Redis Stream
6. Return order ID to client
```

### 4. Matching Engine (`exchange-matching`)

High-performance in-memory order matching.

**Data Structures:**
- Price-time priority queue (red-black tree)
- Order map for O(1) lookup
- Bid/Ask order books per symbol

**Matching Algorithm:**
```go
func Match(incomingOrder Order, book OrderBook) []Trade {
    var trades []Trade

    if incomingOrder.Side == BUY {
        // Match against lowest asks
        for book.HasAsk() && incomingOrder.IsMatched(book.BestAsk()) {
            trade := match(incomingOrder, book.BestAsk())
            trades = append(trades, trade)
        }
    } else {
        // Match against highest bids
        for book.HasBid() && incomingOrder.IsMatched(book.BestBid()) {
            trade := match(incomingOrder, book.BestBid())
            trades = append(trades, trade)
        }
    }

    return trades
}
```

**Performance Characteristics:**
- Single symbol throughput: 10,000+ orders/sec
- Matching latency: <100μs
- Memory usage: ~1MB per million orders

### 5. Clearing Service (`exchange-clearing`)

Manages funds and settlement.

**Responsibilities:**
- Fund freezing and unfreezing
- Trade settlement
- Ledger maintenance
- Balance queries
- Fee calculation and collection

**Double-Entry Bookkeeping:**
```
For each trade:
1. Freeze buyer's quote currency
2. Freeze seller's base currency
3. On match:
   - Deduct frozen funds
   - Credit settled funds
   - Record fee
4. Create ledger entries
```

### 6. Market Data Service (`exchange-marketdata`)

Provides real-time market information.

**Responsibilities:**
- Order book maintenance
- Trade history
- 24h ticker statistics
- WebSocket broadcasting
- REST endpoints for depth/trades/ticker

**WebSocket Channels:**
- `market.{symbol}.depth` - Order book updates
- `market.{symbol}.trades` - Trade executions
- `market.{symbol}.ticker` - 24h statistics
- `private.events` - User-specific events

### 7. Wallet Service (`exchange-wallet`)

Handles deposits and withdrawals.

**Responsibilities:**
- Deposit address generation
- Blockchain monitoring
- Withdrawal processing
- Address whitelisting
- Transaction signing

**State Machine:**
```
Deposits:
PENDING → CONFIRMED → CREDITED → COMPLETED

Withdrawals:
PENDING → APPROVED → PROCESSING → ON_CHAIN → COMPLETED/FAILED
```

### 8. Admin Service (`exchange-admin`)

Administrative operations.

**Responsibilities:**
- RBAC (Role-Based Access Control)
- Trading pair management
- Kill switch (emergency trading halt)
- Audit log viewing
- User management

**Roles:**
- SuperAdmin - Full access
- Operator - Trading pair config
- Support - User support
- Auditor - Read-only access

## 📡 Communication Patterns

### Synchronous (gRPC)

Used for:
- Order → Matching
- Order → Clearing (validation)
- Internal service queries

**Benefits:**
- Strong typing
- High performance
- Schema evolution support

### Asynchronous (Redis Streams)

Used for:
- Order events
- Trade events
- Balance events
- Market data updates

**Benefits:**
- Decoupling
- Backpressure handling
- Replay capability
- Durability

### WebSocket

Used for:
- Real-time order book
- Trade notifications
- Private events
- User order updates

## 💾 Data Storage

### PostgreSQL

**Schemas:**
- `exchange_user` - Users, API keys
- `exchange_order` - Orders, trades, symbols
- `exchange_clearing` - Balances, ledger
- `exchange_wallet` - Deposits, withdrawals
- `exchange_admin` - Audit logs, RBAC

**Best Practices:**
- Connection pooling (PgBouncer recommended)
- Read replicas for query-heavy services
- Regular backups

### Redis

**Use Cases:**
- Caching (trades, ticker)
- Rate limiting counters
- Pub/Sub for real-time updates
- Streams for event queue

**Data Types:**
- Strings - Counters, caches
- Hashes - Order book snapshots
- Sorted Sets - Price levels
- Streams - Event queues

## 🔍 Observability

### Tracing (OpenTelemetry + Jaeger)

Distributed tracing across all services.

**Traced Operations:**
- Order flow end-to-end
- Matching process
- Clearing settlement
- Wallet operations

### Metrics (Prometheus)

**Key Metrics:**
- Request latency (p50, p95, p99)
- Order throughput
- Matching rate
- Error rate
- Queue depth

### Logging (Zerolog)

Structured JSON logging with correlation IDs.

**Log Fields:**
- trace_id
- span_id
- service_name
- level
- message
- context

## 🔐 Security

### Authentication

**Types:**
1. **HMAC-SHA256** - API requests
2. **JWT** - User sessions
3. **Internal Token** - Service-to-service

### Authorization

- API key permissions
- Role-based access control
- IP whitelisting (optional)

### Data Protection

- API key secrets encrypted at rest
- Sensitive data masking in logs
- TLS for all communications
- Rate limiting

## 📈 Performance Optimization

### Matching Engine

- Lock-free data structures
- Batched writes
- Memory-mapped order books
- CPU affinity (optional)

### Network

- HTTP/2 for gRPC
- WebSocket compression
- Connection pooling
- Request multiplexing

### Database

- Prepared statements
- Batch inserts
- Index optimization
- Connection pooling

## 🚀 Scaling Strategies

### Horizontal Scaling

- Stateless services (Gateway, Order, MarketData)
- Share-nothing architecture
- Consistent hashing for order routing

### Vertical Scaling

- Matching engine (CPU-bound)
- Clearing service (I/O-bound)

### Data Partitioning

- By symbol (matching engine)
- By user (order history)
- By time (trades, ledger)
