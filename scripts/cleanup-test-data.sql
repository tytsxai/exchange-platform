-- Cleanup E2E test data (keep system configuration).

-- Orders and trades
DELETE FROM exchange_order.trades
WHERE maker_user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%')
   OR taker_user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%');

DELETE FROM exchange_order.orders
WHERE user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%');

-- Clearing balances and ledger
DELETE FROM exchange_clearing.ledger_entries
WHERE user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%');

DELETE FROM exchange_clearing.account_balances
WHERE user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%');

-- User data
DELETE FROM exchange_user.api_keys
WHERE user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%');

DELETE FROM exchange_user.user_security
WHERE user_id IN (SELECT user_id FROM exchange_user.users WHERE email LIKE 'e2e_%');

DELETE FROM exchange_user.users
WHERE email LIKE 'e2e_%';

-- No sequences to reset (IDs are generated externally).
