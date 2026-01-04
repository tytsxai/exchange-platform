-- Align symbol precision with wallet asset precision.
-- This script updates symbol_configs so that:
--   price_precision == quote asset precision
--   qty_precision   == base asset precision
-- It rescales price_tick/min_notional and qty_step/min_qty/max_qty to preserve real-world values.
--
-- IMPORTANT: Back up the database before running.

WITH cfg AS (
    SELECT
        sc.symbol,
        sc.price_precision,
        sc.qty_precision,
        base.precision AS base_precision,
        quote.precision AS quote_precision
    FROM exchange_order.symbol_configs sc
    JOIN exchange_wallet.assets base ON base.asset = sc.base_asset
    JOIN exchange_wallet.assets quote ON quote.asset = sc.quote_asset
)
UPDATE exchange_order.symbol_configs sc
SET
    price_tick = CASE
        WHEN c.quote_precision >= c.price_precision
            THEN (sc.price_tick::numeric * power(10, c.quote_precision - c.price_precision))::bigint
        ELSE
            (sc.price_tick::numeric / power(10, c.price_precision - c.quote_precision))::bigint
    END,
    min_notional = CASE
        WHEN c.quote_precision >= c.price_precision
            THEN (sc.min_notional::numeric * power(10, c.quote_precision - c.price_precision))::bigint
        ELSE
            (sc.min_notional::numeric / power(10, c.price_precision - c.quote_precision))::bigint
    END,
    qty_step = CASE
        WHEN c.base_precision >= c.qty_precision
            THEN (sc.qty_step::numeric * power(10, c.base_precision - c.qty_precision))::bigint
        ELSE
            (sc.qty_step::numeric / power(10, c.qty_precision - c.base_precision))::bigint
    END,
    min_qty = CASE
        WHEN c.base_precision >= c.qty_precision
            THEN (sc.min_qty::numeric * power(10, c.base_precision - c.qty_precision))::bigint
        ELSE
            (sc.min_qty::numeric / power(10, c.qty_precision - c.base_precision))::bigint
    END,
    max_qty = CASE
        WHEN c.base_precision >= c.qty_precision
            THEN (sc.max_qty::numeric * power(10, c.base_precision - c.qty_precision))::bigint
        ELSE
            (sc.max_qty::numeric / power(10, c.qty_precision - c.base_precision))::bigint
    END,
    price_precision = c.quote_precision,
    qty_precision = c.base_precision,
    updated_at_ms = EXTRACT(EPOCH FROM NOW()) * 1000
FROM cfg c
WHERE sc.symbol = c.symbol
  AND (sc.price_precision <> c.quote_precision OR sc.qty_precision <> c.base_precision);
