-- 验证：账本流水累加 = 当前余额
-- 检查 available 一致性
SELECT
    le.user_id,
    le.asset,
    SUM(le.available_delta) as ledger_available_sum,
    ab.available as balance_available,
    SUM(le.available_delta) - ab.available as available_diff
FROM exchange_clearing.ledger_entries le
JOIN exchange_clearing.account_balances ab
    ON le.user_id = ab.user_id AND le.asset = ab.asset
GROUP BY le.user_id, le.asset, ab.available
HAVING SUM(le.available_delta) != ab.available;

-- 检查 frozen 一致性
SELECT
    le.user_id,
    le.asset,
    SUM(le.frozen_delta) as ledger_frozen_sum,
    ab.frozen as balance_frozen,
    SUM(le.frozen_delta) - ab.frozen as frozen_diff
FROM exchange_clearing.ledger_entries le
JOIN exchange_clearing.account_balances ab
    ON le.user_id = ab.user_id AND le.asset = ab.asset
GROUP BY le.user_id, le.asset, ab.frozen
HAVING SUM(le.frozen_delta) != ab.frozen;
