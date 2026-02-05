# Data Models

Complete reference for OpenExchange data models and database schemas.

## 📊 Database Schemas

OpenExchange uses PostgreSQL with schema-per-service organization:

| Schema | Service | Description |
|--------|---------|-------------|
| `exchange_user` | User | Users, API keys, sessions |
| `exchange_order` | Order | Orders, trades, symbols |
| `exchange_clearing` | Clearing | Balances, ledger entries |
| `exchange_wallet` | Wallet | Deposits, withdrawals |
| `exchange_admin` | Admin | Audit logs, RBAC |
| `exchange_market` | MarketData | Market data, K-lines |

## 👤 User Schema (`exchange_user`)

### Users Table

```sql
CREATE TABLE exchange_user.users (
    id              BIGINT PRIMARY KEY,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'active',
    kyc_status      VARCHAR(32) NOT NULL DEFAULT 'none',
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON exchange_user.users(email);
CREATE INDEX idx_users_status ON exchange_user.users(status);
```

### API Keys Table

```sql
CREATE TABLE exchange_user.api_keys (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES exchange_user.users(id),
    api_key         VARCHAR(64) NOT NULL UNIQUE,
    secret_encrypted VARCHAR(512) NOT NULL,
    permissions     JSONB NOT NULL DEFAULT '[]',
    ip_whitelist    JSONB NOT NULL DEFAULT '[]',
    expire_at       TIMESTAMP,
    last_used_at    TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    revoked_at      TIMESTAMP
);

CREATE INDEX idx_api_keys_user ON exchange_user.api_keys(user_id);
CREATE INDEX idx_api_keys_key ON exchange_user.api_keys(api_key);
```

## 📝 Order Schema (`exchange_order`)

### Orders Table

```sql
CREATE TABLE exchange_order.orders (
    id              BIGINT PRIMARY KEY,           -- Snowflake ID
    user_id         BIGINT NOT NULL,
    symbol          VARCHAR(32) NOT NULL,
    side            VARCHAR(4) NOT NULL,           -- BUY/SELL
    type            VARCHAR(16) NOT NULL,          -- LIMIT/MARKET
    quantity        DECIMAL(32, 18) NOT NULL,
    price           DECIMAL(32, 18),               -- NULL for MARKET
    time_in_force   VARCHAR(8) NOT NULL DEFAULT 'GTC',
    status          VARCHAR(16) NOT NULL,
    filled_qty      DECIMAL(32, 18) NOT NULL DEFAULT '0',
    avg_price       DECIMAL(32, 18) NOT NULL DEFAULT '0',
    fee             DECIMAL(32, 18) NOT NULL DEFAULT '0',
    fee_currency    VARCHAR(16),
    source          VARCHAR(32),                   -- API/WEB/APP
    client_order_id VARCHAR(64),
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    expired_at      TIMESTAMP
);

CREATE INDEX idx_orders_user_symbol ON exchange_order.orders(user_id, symbol);
CREATE INDEX idx_orders_status ON exchange_order.orders(status);
CREATE INDEX idx_orders_created_at ON exchange_order.orders(created_at DESC);
CREATE INDEX idx_orders_user_status ON exchange_order.orders(user_id, status);
```

### Trades Table

```sql
CREATE TABLE exchange_order.trades (
    id              BIGINT PRIMARY KEY,
    order_id        BIGINT NOT NULL REFERENCES exchange_order.orders(id),
    symbol          VARCHAR(32) NOT NULL,
    side            VARCHAR(4) NOT NULL,
    price           DECIMAL(32, 18) NOT NULL,
    quantity        DECIMAL(32, 18) NOT NULL,
    maker_order_id  BIGINT NOT NULL,
    taker_order_id  BIGINT NOT NULL,
    maker_user_id   BIGINT NOT NULL,
    taker_user_id   BIGINT NOT NULL,
    fee             DECIMAL(32, 18) NOT NULL,
    fee_currency    VARCHAR(16) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trades_symbol ON exchange_order.trades(symbol);
CREATE INDEX idx_trades_order ON exchange_order.trades(order_id);
CREATE INDEX idx_trades_created_at ON exchange_order.trades(created_at DESC);
```

### Symbol Config Table

```sql
CREATE TABLE exchange_order.symbols (
    id              BIGINT PRIMARY KEY,
    symbol          VARCHAR(32) NOT NULL UNIQUE,
    base_currency   VARCHAR(16) NOT NULL,
    quote_currency  VARCHAR(16) NOT NULL,
    price_precision INT NOT NULL DEFAULT 8,
    qty_precision   INT NOT NULL DEFAULT 8,
    min_qty        DECIMAL(32, 18) NOT NULL DEFAULT '0.0001',
    min_price      DECIMAL(32, 18) NOT NULL DEFAULT '0.01',
    status         VARCHAR(16) NOT NULL DEFAULT 'enabled',
    created_at     TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP NOT NULL DEFAULT NOW()
);
```

## 💰 Clearing Schema (`exchange_clearing`)

### Balances Table

```sql
CREATE TABLE exchange_clearing.balances (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    asset           VARCHAR(16) NOT NULL,
    available       DECIMAL(32, 18) NOT NULL DEFAULT '0',
    frozen          DECIMAL(32, 18) NOT NULL DEFAULT '0',
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, asset)
);

CREATE INDEX idx_balances_user ON exchange_clearing.balances(user_id);
```

### Ledger Table

```sql
CREATE TABLE exchange_clearing.ledger (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    asset           VARCHAR(16) NOT NULL,
    amount          DECIMAL(32, 18) NOT NULL,
    balance_after   DECIMAL(32, 18) NOT NULL,
    reason          VARCHAR(32) NOT NULL,
    order_id        BIGINT,
    trade_id        BIGINT,
    tx_hash         VARCHAR(256),
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ledger_user ON exchange_clearing.ledger(user_id);
CREATE INDEX idx_ledger_asset ON exchange_clearing.ledger(asset);
CREATE INDEX idx_ledger_order ON exchange_clearing.ledger(order_id);
CREATE INDEX idx_ledger_created ON exchange_clearing.ledger(created_at DESC);
```

## 💼 Wallet Schema (`exchange_wallet`)

### Deposits Table

```sql
CREATE TABLE exchange_wallet.deposits (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    asset           VARCHAR(16) NOT NULL,
    amount          DECIMAL(32, 18) NOT NULL,
    tx_hash         VARCHAR(256) NOT NULL,
    from_address    VARCHAR(256),
    confirmations   INT NOT NULL DEFAULT 0,
    required_confirmations INT NOT NULL DEFAULT 6,
    status          VARCHAR(32) NOT NULL,
    credited_at     TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deposits_user ON exchange_wallet.deposits(user_id);
CREATE INDEX idx_deposits_tx ON exchange_wallet.deposits(tx_hash);
CREATE INDEX idx_deposits_status ON exchange_wallet.deposits(status);
```

### Withdrawals Table

```sql
CREATE TABLE exchange_wallet.withdrawals (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    asset           VARCHAR(16) NOT NULL,
    amount          DECIMAL(32, 18) NOT NULL,
    fee             DECIMAL(32, 18) NOT NULL,
    to_address      VARCHAR(256) NOT NULL,
    tx_hash         VARCHAR(256),
    status          VARCHAR(32) NOT NULL,
    approved_by     BIGINT,
    approved_at     TIMESTAMP,
    processed_at    TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_withdrawals_user ON exchange_wallet.withdrawals(user_id);
CREATE INDEX idx_withdrawals_status ON exchange_wallet.withdrawals(status);
CREATE INDEX idx_withdrawals_tx ON exchange_wallet.withdrawals(tx_hash);
```

### Network Config Table

```sql
CREATE TABLE exchange_wallet.networks (
    id              BIGINT PRIMARY KEY,
    asset           VARCHAR(16) NOT NULL,
    network         VARCHAR(32) NOT NULL,
    explorer_url    VARCHAR(256),
    min_deposit     DECIMAL(32, 18) NOT NULL DEFAULT '0',
    min_withdraw    DECIMAL(32, 18) NOT NULL DEFAULT '0',
    withdraw_fee    DECIMAL(32, 18) NOT NULL DEFAULT '0',
    confirmations   INT NOT NULL DEFAULT 6,
    status          VARCHAR(16) NOT NULL DEFAULT 'enabled',
    UNIQUE(asset, network)
);
```

## 📊 Admin Schema (`exchange_admin`)

### Audit Logs Table

```sql
CREATE TABLE exchange_admin.audit_logs (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT,
    event_type      VARCHAR(64) NOT NULL,
    event_action    VARCHAR(64) NOT NULL,
    resource_type   VARCHAR(64),
    resource_id     VARCHAR(128),
    ip_address      VARCHAR(64),
    user_agent      VARCHAR(512),
    request_params  JSONB,
    response_status VARCHAR(32),
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_user ON exchange_admin.audit_logs(user_id);
CREATE INDEX idx_audit_event ON exchange_admin.audit_logs(event_type);
CREATE INDEX idx_audit_created ON exchange_admin.audit_logs(created_at DESC);
```

### Roles Table

```sql
CREATE TABLE exchange_admin.roles (
    id              BIGINT PRIMARY KEY,
    name            VARCHAR(64) NOT NULL UNIQUE,
    description     VARCHAR(256),
    permissions     JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE exchange_admin.role_assignments (
    id              BIGINT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    role_id         BIGINT NOT NULL REFERENCES exchange_admin.roles(id),
    assigned_at     TIMESTAMP NOT NULL DEFAULT NOW()
);
```

## 📈 Proto Definitions

### Order Message

```protobuf
message Order {
    string id = 1;
    string user_id = 2;
    string symbol = 3;
    Side side = 4;
    OrderType type = 5;
    string quantity = 6;
    string price = 7;
    TimeInForce time_in_force = 8;
    OrderStatus status = 9;
    string filled_qty = 10;
    string avg_price = 11;
    string fee = 12;
    string fee_currency = 13;
    int64 created_at = 14;
}

enum Side {
    BUY = 0;
    SELL = 1;
}

enum OrderType {
    LIMIT = 0;
    MARKET = 1;
}

enum OrderStatus {
    INIT = 0;
    NEW = 1;
    PARTIALLY_FILLED = 2;
    FILLED = 3;
    CANCELED = 4;
    REJECTED = 5;
    EXPIRED = 6;
}
```

### Trade Message

```protobuf
message Trade {
    string id = 1;
    string symbol = 2;
    string price = 3;
    string quantity = 4;
    string maker_order_id = 5;
    string taker_order_id = 6;
    string maker_user_id = 7;
    string taker_user_id = 8;
    string fee = 9;
    string fee_currency = 10;
    int64 created_at = 11;
}
```

### Balance Message

```protobuf
message Balance {
    string asset = 1;
    string available = 2;
    string frozen = 3;
}

message LedgerEntry {
    string id = 1;
    string asset = 2;
    string amount = 3;
    string balance_after = 4;
    string reason = 5;
    string order_id = 6;
    string trade_id = 7;
    int64 created_at = 8;
}
```

## 🔗 Related Documentation

- [Architecture](architecture.md) - System architecture
- [API Documentation](api.md) - REST API reference
- [Event Model](event-model.md) - Event-driven communication
