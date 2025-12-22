-- Migration: scale DECIMAL numeric fields to BIGINT scaled integers.
-- IMPORTANT: Run this once on the legacy decimal schema before applying BIGINT column types.
-- Assumptions:
--   1) Old values are stored in human units (e.g., price=30000.12, qty=0.01).
--   2) symbol_configs.price_precision/qty_precision and assets.precision are populated.
--   3) After scaling, columns will be cast to BIGINT.

BEGIN;

-- 1) exchange_order.symbol_configs
UPDATE exchange_order.symbol_configs
SET price_tick = price_tick * power(10, price_precision),
    qty_step = qty_step * power(10, qty_precision),
    min_qty = min_qty * power(10, qty_precision),
    max_qty = max_qty * power(10, qty_precision),
    min_notional = min_notional * power(10, price_precision);

ALTER TABLE exchange_order.symbol_configs
    ALTER COLUMN price_tick TYPE BIGINT USING price_tick::BIGINT,
    ALTER COLUMN qty_step TYPE BIGINT USING qty_step::BIGINT,
    ALTER COLUMN min_qty TYPE BIGINT USING min_qty::BIGINT,
    ALTER COLUMN max_qty TYPE BIGINT USING max_qty::BIGINT,
    ALTER COLUMN min_notional TYPE BIGINT USING min_notional::BIGINT;

-- 2) exchange_order.orders
UPDATE exchange_order.orders o
SET price = o.price * power(10, sc.price_precision),
    stop_price = o.stop_price * power(10, sc.price_precision),
    orig_qty = o.orig_qty * power(10, sc.qty_precision),
    executed_qty = o.executed_qty * power(10, sc.qty_precision),
    cumulative_quote_qty = o.cumulative_quote_qty * power(10, sc.price_precision)
FROM exchange_order.symbol_configs sc
WHERE o.symbol = sc.symbol;

ALTER TABLE exchange_order.orders
    ALTER COLUMN price TYPE BIGINT USING price::BIGINT,
    ALTER COLUMN stop_price TYPE BIGINT USING stop_price::BIGINT,
    ALTER COLUMN orig_qty TYPE BIGINT USING orig_qty::BIGINT,
    ALTER COLUMN executed_qty TYPE BIGINT USING executed_qty::BIGINT,
    ALTER COLUMN cumulative_quote_qty TYPE BIGINT USING cumulative_quote_qty::BIGINT;

-- 3) exchange_order.trades
UPDATE exchange_order.trades t
SET price = t.price * power(10, sc.price_precision),
    qty = t.qty * power(10, sc.qty_precision),
    quote_qty = t.quote_qty * power(10, sc.price_precision)
FROM exchange_order.symbol_configs sc
WHERE t.symbol = sc.symbol;

UPDATE exchange_order.trades t
SET maker_fee = t.maker_fee * power(10, a.precision),
    taker_fee = t.taker_fee * power(10, a.precision)
FROM exchange_wallet.assets a
WHERE t.fee_asset = a.asset;

ALTER TABLE exchange_order.trades
    ALTER COLUMN price TYPE BIGINT USING price::BIGINT,
    ALTER COLUMN qty TYPE BIGINT USING qty::BIGINT,
    ALTER COLUMN quote_qty TYPE BIGINT USING quote_qty::BIGINT,
    ALTER COLUMN maker_fee TYPE BIGINT USING maker_fee::BIGINT,
    ALTER COLUMN taker_fee TYPE BIGINT USING taker_fee::BIGINT;

-- 4) exchange_clearing.account_balances
UPDATE exchange_clearing.account_balances b
SET available = b.available * power(10, a.precision),
    frozen = b.frozen * power(10, a.precision)
FROM exchange_wallet.assets a
WHERE b.asset = a.asset;

ALTER TABLE exchange_clearing.account_balances
    ALTER COLUMN available TYPE BIGINT USING available::BIGINT,
    ALTER COLUMN frozen TYPE BIGINT USING frozen::BIGINT;

-- 5) exchange_clearing.ledger_entries
UPDATE exchange_clearing.ledger_entries l
SET available_delta = l.available_delta * power(10, a.precision),
    frozen_delta = l.frozen_delta * power(10, a.precision),
    available_after = l.available_after * power(10, a.precision),
    frozen_after = l.frozen_after * power(10, a.precision)
FROM exchange_wallet.assets a
WHERE l.asset = a.asset;

ALTER TABLE exchange_clearing.ledger_entries
    ALTER COLUMN available_delta TYPE BIGINT USING available_delta::BIGINT,
    ALTER COLUMN frozen_delta TYPE BIGINT USING frozen_delta::BIGINT,
    ALTER COLUMN available_after TYPE BIGINT USING available_after::BIGINT,
    ALTER COLUMN frozen_after TYPE BIGINT USING frozen_after::BIGINT;

-- 6) exchange_market.klines
UPDATE exchange_market.klines k
SET open = k.open * power(10, sc.price_precision),
    high = k.high * power(10, sc.price_precision),
    low = k.low * power(10, sc.price_precision),
    close = k.close * power(10, sc.price_precision),
    volume = k.volume * power(10, sc.qty_precision),
    quote_volume = k.quote_volume * power(10, sc.price_precision)
FROM exchange_order.symbol_configs sc
WHERE k.symbol = sc.symbol;

ALTER TABLE exchange_market.klines
    ALTER COLUMN open TYPE BIGINT USING open::BIGINT,
    ALTER COLUMN high TYPE BIGINT USING high::BIGINT,
    ALTER COLUMN low TYPE BIGINT USING low::BIGINT,
    ALTER COLUMN close TYPE BIGINT USING close::BIGINT,
    ALTER COLUMN volume TYPE BIGINT USING volume::BIGINT,
    ALTER COLUMN quote_volume TYPE BIGINT USING quote_volume::BIGINT;

-- 7) exchange_wallet.networks
UPDATE exchange_wallet.networks n
SET min_withdraw = n.min_withdraw * power(10, a.precision),
    withdraw_fee = n.withdraw_fee * power(10, a.precision)
FROM exchange_wallet.assets a
WHERE n.asset = a.asset;

ALTER TABLE exchange_wallet.networks
    ALTER COLUMN min_withdraw TYPE BIGINT USING min_withdraw::BIGINT,
    ALTER COLUMN withdraw_fee TYPE BIGINT USING withdraw_fee::BIGINT;

-- 8) exchange_wallet.deposits
UPDATE exchange_wallet.deposits d
SET amount = d.amount * power(10, a.precision)
FROM exchange_wallet.assets a
WHERE d.asset = a.asset;

ALTER TABLE exchange_wallet.deposits
    ALTER COLUMN amount TYPE BIGINT USING amount::BIGINT;

-- 9) exchange_wallet.withdrawals
UPDATE exchange_wallet.withdrawals w
SET amount = w.amount * power(10, a.precision),
    fee = w.fee * power(10, a.precision)
FROM exchange_wallet.assets a
WHERE w.asset = a.asset;

ALTER TABLE exchange_wallet.withdrawals
    ALTER COLUMN amount TYPE BIGINT USING amount::BIGINT,
    ALTER COLUMN fee TYPE BIGINT USING fee::BIGINT;

COMMIT;
