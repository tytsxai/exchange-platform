-- 初始化数据库 schema

-- 创建各服务的 schema
CREATE SCHEMA IF NOT EXISTS exchange_user;
CREATE SCHEMA IF NOT EXISTS exchange_order;
CREATE SCHEMA IF NOT EXISTS exchange_clearing;
CREATE SCHEMA IF NOT EXISTS exchange_market;
CREATE SCHEMA IF NOT EXISTS exchange_wallet;
CREATE SCHEMA IF NOT EXISTS exchange_admin;

-- 启用扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ========== exchange_user schema ==========

-- 用户表
CREATE TABLE exchange_user.users (
    user_id BIGINT PRIMARY KEY,
    email VARCHAR(255) UNIQUE,
    phone VARCHAR(50) UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    status SMALLINT NOT NULL DEFAULT 1,  -- 1=ACTIVE, 2=FROZEN, 3=DISABLED
    kyc_status SMALLINT NOT NULL DEFAULT 1,  -- 1=NOT_STARTED, 2=IN_REVIEW, 3=APPROVED, 4=REJECTED
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL
);

CREATE INDEX idx_users_email ON exchange_user.users(email);
CREATE INDEX idx_users_phone ON exchange_user.users(phone);

-- 用户安全设置
CREATE TABLE exchange_user.user_security (
    user_id BIGINT PRIMARY KEY REFERENCES exchange_user.users(user_id),
    totp_secret VARCHAR(64),
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    withdraw_whitelist_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    anti_phishing_code VARCHAR(32),
    updated_at_ms BIGINT NOT NULL
);

-- API Key 表
CREATE TABLE exchange_user.api_keys (
    api_key_id BIGINT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES exchange_user.users(user_id),
    api_key VARCHAR(64) UNIQUE NOT NULL,
    secret_hash VARCHAR(255) NOT NULL,
    label VARCHAR(100),
    permissions INTEGER NOT NULL DEFAULT 1,  -- bitmask: 1=READ, 2=TRADE, 4=WITHDRAW
    ip_whitelist TEXT[],
    status SMALLINT NOT NULL DEFAULT 1,  -- 1=ACTIVE, 2=DISABLED
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL,
    last_used_at_ms BIGINT
);

CREATE INDEX idx_api_keys_user_id ON exchange_user.api_keys(user_id);
CREATE INDEX idx_api_keys_api_key ON exchange_user.api_keys(api_key);

-- ========== exchange_order schema ==========

-- 交易对配置
CREATE TABLE exchange_order.symbol_configs (
    symbol VARCHAR(32) PRIMARY KEY,
    base_asset VARCHAR(16) NOT NULL,
    quote_asset VARCHAR(16) NOT NULL,
    price_tick NUMERIC(32, 16) NOT NULL,
    qty_step NUMERIC(32, 16) NOT NULL,
    price_precision SMALLINT NOT NULL,
    qty_precision SMALLINT NOT NULL,
    min_qty NUMERIC(32, 16) NOT NULL,
    max_qty NUMERIC(32, 16) NOT NULL,
    min_notional NUMERIC(32, 16) NOT NULL,
    price_limit_rate NUMERIC(8, 4) NOT NULL DEFAULT 0.05,
    maker_fee_rate NUMERIC(8, 6) NOT NULL DEFAULT 0.001,
    taker_fee_rate NUMERIC(8, 6) NOT NULL DEFAULT 0.001,
    status SMALLINT NOT NULL DEFAULT 1,  -- 1=TRADING, 2=HALT, 3=CANCEL_ONLY
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL
);

-- 订单表
CREATE TABLE exchange_order.orders (
    order_id BIGINT PRIMARY KEY,
    client_order_id VARCHAR(64),
    user_id BIGINT NOT NULL,
    symbol VARCHAR(32) NOT NULL,
    side SMALLINT NOT NULL,  -- 1=BUY, 2=SELL
    type SMALLINT NOT NULL,  -- 1=LIMIT, 2=MARKET
    time_in_force SMALLINT NOT NULL DEFAULT 1,  -- 1=GTC, 2=IOC, 3=FOK, 4=POST_ONLY
    price NUMERIC(32, 16),
    stop_price NUMERIC(32, 16),
    orig_qty NUMERIC(32, 16) NOT NULL,
    executed_qty NUMERIC(32, 16) NOT NULL DEFAULT 0,
    cumulative_quote_qty NUMERIC(32, 16) NOT NULL DEFAULT 0,
    status SMALLINT NOT NULL DEFAULT 1,  -- 1=NEW, 2=PARTIAL, 3=FILLED, 4=CANCELED, 5=REJECTED, 6=EXPIRED
    reject_reason VARCHAR(255),
    cancel_reason VARCHAR(255),
    stp_mode SMALLINT NOT NULL DEFAULT 1,  -- 1=NONE, 2=CANCEL_TAKER, 3=CANCEL_MAKER, 4=CANCEL_BOTH
    create_time_ms BIGINT NOT NULL,
    update_time_ms BIGINT NOT NULL,
    transact_time_ms BIGINT,
    UNIQUE(user_id, client_order_id)
);

CREATE INDEX idx_orders_user_status ON exchange_order.orders(user_id, status, update_time_ms DESC);
CREATE INDEX idx_orders_user_symbol ON exchange_order.orders(user_id, symbol, update_time_ms DESC);
CREATE INDEX idx_orders_symbol ON exchange_order.orders(symbol, update_time_ms DESC);

-- 成交表
CREATE TABLE exchange_order.trades (
    trade_id BIGINT PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    maker_order_id BIGINT NOT NULL,
    taker_order_id BIGINT NOT NULL,
    maker_user_id BIGINT NOT NULL,
    taker_user_id BIGINT NOT NULL,
    price NUMERIC(32, 16) NOT NULL,
    qty NUMERIC(32, 16) NOT NULL,
    quote_qty NUMERIC(32, 16) NOT NULL,
    maker_fee NUMERIC(32, 16) NOT NULL,
    taker_fee NUMERIC(32, 16) NOT NULL,
    fee_asset VARCHAR(16) NOT NULL,
    taker_side SMALLINT NOT NULL,
    timestamp_ms BIGINT NOT NULL
);

CREATE INDEX idx_trades_symbol ON exchange_order.trades(symbol, timestamp_ms DESC);
CREATE INDEX idx_trades_maker_order ON exchange_order.trades(maker_order_id);
CREATE INDEX idx_trades_taker_order ON exchange_order.trades(taker_order_id);
CREATE INDEX idx_trades_maker_user ON exchange_order.trades(maker_user_id, timestamp_ms DESC);
CREATE INDEX idx_trades_taker_user ON exchange_order.trades(taker_user_id, timestamp_ms DESC);

-- ========== exchange_clearing schema ==========

-- 账户余额
CREATE TABLE exchange_clearing.account_balances (
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    available NUMERIC(32, 16) NOT NULL DEFAULT 0,
    frozen NUMERIC(32, 16) NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    updated_at_ms BIGINT NOT NULL,
    PRIMARY KEY (user_id, asset),
    CONSTRAINT chk_available_non_negative CHECK (available >= 0),
    CONSTRAINT chk_frozen_non_negative CHECK (frozen >= 0)
);

-- 账本流水（资金 truth）
CREATE TABLE exchange_clearing.ledger_entries (
    ledger_id BIGINT PRIMARY KEY,
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    available_delta NUMERIC(32, 16) NOT NULL,
    frozen_delta NUMERIC(32, 16) NOT NULL,
    available_after NUMERIC(32, 16) NOT NULL,
    frozen_after NUMERIC(32, 16) NOT NULL,
    reason SMALLINT NOT NULL,  -- 1=ORDER_FREEZE, 2=ORDER_UNFREEZE, 3=TRADE_SETTLE, 4=FEE, 5=DEPOSIT, 6=WITHDRAW...
    ref_type VARCHAR(32) NOT NULL,
    ref_id VARCHAR(64) NOT NULL,
    note TEXT,
    created_at_ms BIGINT NOT NULL
);

CREATE INDEX idx_ledger_user_asset ON exchange_clearing.ledger_entries(user_id, asset, created_at_ms DESC);
CREATE INDEX idx_ledger_ref ON exchange_clearing.ledger_entries(ref_type, ref_id);

-- ========== exchange_market schema ==========

-- K 线数据
CREATE TABLE exchange_market.klines (
    symbol VARCHAR(32) NOT NULL,
    interval SMALLINT NOT NULL,  -- 1=1m, 2=5m, 3=15m, 4=30m, 5=1h, 6=4h, 7=1d, 8=1w
    open_time_ms BIGINT NOT NULL,
    close_time_ms BIGINT NOT NULL,
    open NUMERIC(32, 16) NOT NULL,
    high NUMERIC(32, 16) NOT NULL,
    low NUMERIC(32, 16) NOT NULL,
    close NUMERIC(32, 16) NOT NULL,
    volume NUMERIC(32, 16) NOT NULL,
    quote_volume NUMERIC(32, 16) NOT NULL,
    trade_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (symbol, interval, open_time_ms)
);

-- ========== exchange_wallet schema ==========

-- 资产配置
CREATE TABLE exchange_wallet.assets (
    asset VARCHAR(16) PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    precision SMALLINT NOT NULL,
    status SMALLINT NOT NULL DEFAULT 1,
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL
);

-- 网络配置
CREATE TABLE exchange_wallet.networks (
    asset VARCHAR(16) NOT NULL,
    network VARCHAR(32) NOT NULL,
    deposit_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    withdraw_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    min_withdraw NUMERIC(32, 16) NOT NULL,
    withdraw_fee NUMERIC(32, 16) NOT NULL,
    confirmations_required INT NOT NULL DEFAULT 6,
    contract_address VARCHAR(128),
    status SMALLINT NOT NULL DEFAULT 1,
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL,
    PRIMARY KEY (asset, network)
);

-- 充值地址
CREATE TABLE exchange_wallet.deposit_addresses (
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    network VARCHAR(32) NOT NULL,
    address VARCHAR(128) NOT NULL,
    tag VARCHAR(64),
    created_at_ms BIGINT NOT NULL,
    PRIMARY KEY (user_id, asset, network)
);

-- 充值记录
CREATE TABLE exchange_wallet.deposits (
    deposit_id BIGINT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    network VARCHAR(32) NOT NULL,
    amount NUMERIC(32, 16) NOT NULL,
    txid VARCHAR(128) NOT NULL,
    vout INT NOT NULL DEFAULT 0,
    confirmations INT NOT NULL DEFAULT 0,
    status SMALLINT NOT NULL DEFAULT 1,  -- 1=PENDING, 2=CONFIRMED, 3=CREDITED
    credited_at_ms BIGINT,
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL,
    UNIQUE (asset, network, txid, vout)
);

CREATE INDEX idx_deposits_user ON exchange_wallet.deposits(user_id, created_at_ms DESC);

-- 提现记录
CREATE TABLE exchange_wallet.withdrawals (
    withdraw_id BIGINT PRIMARY KEY,
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    network VARCHAR(32) NOT NULL,
    amount NUMERIC(32, 16) NOT NULL,
    fee NUMERIC(32, 16) NOT NULL,
    address VARCHAR(128) NOT NULL,
    tag VARCHAR(64),
    status SMALLINT NOT NULL DEFAULT 1,  -- 1=PENDING, 2=APPROVED, 3=REJECTED, 4=PROCESSING, 5=COMPLETED, 6=FAILED
    txid VARCHAR(128),
    requested_at_ms BIGINT NOT NULL,
    approved_at_ms BIGINT,
    approved_by BIGINT,
    sent_at_ms BIGINT,
    completed_at_ms BIGINT
);

CREATE INDEX idx_withdrawals_user ON exchange_wallet.withdrawals(user_id, requested_at_ms DESC);
CREATE INDEX idx_withdrawals_status ON exchange_wallet.withdrawals(status, requested_at_ms);

-- 地址白名单
CREATE TABLE exchange_wallet.address_book (
    address_book_id BIGINT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    asset VARCHAR(16) NOT NULL,
    network VARCHAR(32) NOT NULL,
    address VARCHAR(128) NOT NULL,
    tag VARCHAR(64),
    label VARCHAR(100),
    created_at_ms BIGINT NOT NULL
);

CREATE INDEX idx_address_book_user ON exchange_wallet.address_book(user_id);

-- ========== exchange_admin schema ==========

-- 审计日志
CREATE TABLE exchange_admin.audit_logs (
    audit_id BIGINT PRIMARY KEY,
    actor_user_id BIGINT NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(32) NOT NULL,
    target_id VARCHAR(64),
    before_json JSONB,
    after_json JSONB,
    ip VARCHAR(45),
    created_at_ms BIGINT NOT NULL
);

CREATE INDEX idx_audit_logs_actor ON exchange_admin.audit_logs(actor_user_id, created_at_ms DESC);
CREATE INDEX idx_audit_logs_target ON exchange_admin.audit_logs(target_type, target_id, created_at_ms DESC);

-- 角色表
CREATE TABLE exchange_admin.roles (
    role_id BIGINT PRIMARY KEY,
    name VARCHAR(64) UNIQUE NOT NULL,
    permissions TEXT[] NOT NULL,
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL
);

-- 用户角色关联
CREATE TABLE exchange_admin.user_roles (
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL REFERENCES exchange_admin.roles(role_id),
    created_at_ms BIGINT NOT NULL,
    PRIMARY KEY (user_id, role_id)
);

-- ========== 初始数据 ==========

-- 插入默认交易对
INSERT INTO exchange_order.symbol_configs (symbol, base_asset, quote_asset, price_tick, qty_step, price_precision, qty_precision, min_qty, max_qty, min_notional, maker_fee_rate, taker_fee_rate, status, created_at_ms, updated_at_ms)
VALUES
    ('BTCUSDT', 'BTC', 'USDT', 0.01, 0.0001, 2, 4, 0.0001, 1000, 10, 0.001, 0.001, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('ETHUSDT', 'ETH', 'USDT', 0.01, 0.001, 2, 3, 0.001, 10000, 10, 0.001, 0.001, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('SOLUSDT', 'SOL', 'USDT', 0.001, 0.01, 3, 2, 0.01, 100000, 5, 0.001, 0.001, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000);

-- 插入默认资产
INSERT INTO exchange_wallet.assets (asset, name, precision, status, created_at_ms, updated_at_ms)
VALUES
    ('BTC', 'Bitcoin', 8, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('ETH', 'Ethereum', 8, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('SOL', 'Solana', 8, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('USDT', 'Tether USD', 6, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000);

-- 插入默认管理员角色
INSERT INTO exchange_admin.roles (role_id, name, permissions, created_at_ms, updated_at_ms)
VALUES
    (1, 'super_admin', ARRAY['*'], EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    (2, 'operator', ARRAY['symbol:read', 'symbol:write', 'fee:read', 'fee:write', 'risk:read', 'risk:write'], EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    (3, 'auditor', ARRAY['audit:read', 'user:read', 'order:read', 'trade:read', 'ledger:read'], EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000);
