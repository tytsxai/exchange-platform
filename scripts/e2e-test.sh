#!/bin/bash
set -euo pipefail

# Phase 1 E2E tests

# Config
API_URL=${API_URL:-"http://localhost:8080"}
USER_URL=${USER_URL:-"http://localhost:8085"}
ORDER_URL=${ORDER_URL:-"http://localhost:8081"}
MATCHING_URL=${MATCHING_URL:-"http://localhost:8082"}
CLEARING_URL=${CLEARING_URL:-"http://localhost:8083"}
WS_URL=${WS_URL:-"ws://localhost:8080"}
DB_URL=${DB_URL:-"postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"}

GATEWAY_API_KEY=${GATEWAY_API_KEY:-"test-api-key"}
GATEWAY_API_SECRET=${GATEWAY_API_SECRET:-"test-secret"}
WS_USER_ID=${WS_USER_ID:-1}
AUTO_CLEANUP=${AUTO_CLEANUP:-1}

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/test-helpers.sh"

cleanup_data() {
    if [ -f "${SCRIPT_DIR}/cleanup-test-data.sql" ]; then
        log_info "Cleaning up test data..."
        require_cmd psql
        psql -X "${DB_URL}" -v ON_ERROR_STOP=1 -q -f "${SCRIPT_DIR}/cleanup-test-data.sql"
    fi
}

parse_order_id() {
    local resp=$1
    python3 - "$resp" <<'PY'
import json,sys
resp=json.loads(sys.argv[1])
for key in ("OrderID","orderId","orderID"):
    if key in resp:
        print(resp[key])
        sys.exit(0)
print("")
PY
}

assert_non_empty() {
    local value=$1
    local message=$2
    if [ -z "$value" ]; then
        log_error "$message"
        return 1
    fi
}

test_deposit_and_order() {
    log_info "Test: Deposit and order flow"

    local user_id=$WS_USER_ID
    local asset="USDT"
    local symbol="BTCUSDT"
    local deposit_amount
    deposit_amount=$(scale_asset_amount "$asset" "10000")

    deposit "$user_id" "$asset" "$deposit_amount"

    local before_frozen
    before_frozen=$(check_balance gateway "$user_id" "$asset" "Frozen")

    local price="30000"
    local qty="0.1"
    local client_id="e2e-deposit-order-$(now_ms)"
    local resp
    resp=$(place_order gateway "$user_id" "$symbol" "BUY" "LIMIT" "$price" "$qty" "$client_id")

    local order_id
    order_id=$(parse_order_id "$resp")
    assert_non_empty "$order_id" "Order ID not found in response: $resp"

    local after_frozen
    after_frozen=$(check_balance gateway "$user_id" "$asset" "Frozen")

    local expected_freeze
    expected_freeze=$(quote_amount "$symbol" "$price" "$qty")
    if [ -z "$expected_freeze" ]; then
        log_error "Failed to calculate expected freeze amount"
        return 1
    fi
    if [ -n "$before_frozen" ] && [ -n "$after_frozen" ]; then
        local delta=$((after_frozen - before_frozen))
        if [ "$delta" -ne "$expected_freeze" ]; then
            log_error "Frozen balance mismatch: expected ${expected_freeze}, got ${delta}"
            return 1
        fi
    fi

    local status
    status=$(check_order_status gateway "$user_id" "$order_id")
    if [ "$status" != "1" ] && [ "$status" != "NEW" ]; then
        log_error "Unexpected order status: $status"
        return 1
    fi

    cancel_order gateway "$user_id" "BTCUSDT" "$order_id" >/dev/null
}

test_matching() {
    log_info "Test: Matching trade flow"

    local ts
    ts=$(now_ms)
    local user_a
    local user_b
    user_a=$(create_test_user "e2e_user_a_${ts}@local")
    user_b=$(create_test_user "e2e_user_b_${ts}@local")

    assert_non_empty "$user_a" "User A not created"
    assert_non_empty "$user_b" "User B not created"

    deposit "$user_a" "BTC" "$(scale_asset_amount "BTC" "1")"
    deposit "$user_b" "USDT" "$(scale_asset_amount "USDT" "10000")"

    local seller_btc_before
    local buyer_usdt_before
    seller_btc_before=$(check_balance direct "$user_a" "BTC" "Available")
    buyer_usdt_before=$(check_balance direct "$user_b" "USDT" "Available")

    local price="31000"
    local qty="0.1"

    local sell_resp
    sell_resp=$(place_order direct "$user_a" "BTCUSDT" "SELL" "LIMIT" "$price" "$qty" "e2e-sell-${ts}")
    local sell_id
    sell_id=$(parse_order_id "$sell_resp")
    assert_non_empty "$sell_id" "Sell order id missing"

    local buy_resp
    buy_resp=$(place_order direct "$user_b" "BTCUSDT" "BUY" "LIMIT" "$price" "$qty" "e2e-buy-${ts}")
    local buy_id
    buy_id=$(parse_order_id "$buy_resp")
    assert_non_empty "$buy_id" "Buy order id missing"

    # Wait for trade to be created
    local trade_count=0
    for i in {1..30}; do
        trade_count=$(psql_exec "SELECT COUNT(*) FROM exchange_order.trades WHERE (maker_user_id=${user_a} AND taker_user_id=${user_b}) OR (maker_user_id=${user_b} AND taker_user_id=${user_a});" | tr -d ' ' || echo "0")
        trade_count=${trade_count:-0}
        if [ "$trade_count" -gt 0 ] 2>/dev/null; then
            break
        fi
        sleep 1
    done

    local seller_btc_after
    local buyer_usdt_after
    seller_btc_after=$(check_balance direct "$user_a" "BTC" "Available")
    buyer_usdt_after=$(check_balance direct "$user_b" "USDT" "Available")

    if [ -n "$seller_btc_before" ] && [ -n "$seller_btc_after" ]; then
        if [ "$seller_btc_after" -ge "$seller_btc_before" ]; then
            log_error "Seller BTC did not decrease"
            return 1
        fi
    fi

    if [ -n "$buyer_usdt_before" ] && [ -n "$buyer_usdt_after" ]; then
        if [ "$buyer_usdt_after" -ge "$buyer_usdt_before" ]; then
            log_error "Buyer USDT did not decrease"
            return 1
        fi
    fi

    local ledger_count_a
    local ledger_count_b
    ledger_count_a=$(psql_exec "SELECT COUNT(*) FROM exchange_clearing.ledger_entries WHERE user_id=${user_a} AND ref_type='TRADE';")
    ledger_count_b=$(psql_exec "SELECT COUNT(*) FROM exchange_clearing.ledger_entries WHERE user_id=${user_b} AND ref_type='TRADE';")

    if [ "$ledger_count_a" -le 0 ] || [ "$ledger_count_b" -le 0 ]; then
        log_error "Ledger entries for trade not found"
        return 1
    fi
}

test_cancel_order() {
    log_info "Test: Cancel order flow"

    local user_id=$WS_USER_ID
    local asset="USDT"

    deposit "$user_id" "$asset" "$(scale_asset_amount "$asset" "5000")"

    local before_frozen
    before_frozen=$(check_balance gateway "$user_id" "$asset" "Frozen")

    local price="10000"
    local qty="0.1"
    local client_id="e2e-cancel-$(now_ms)"

    local resp
    resp=$(place_order gateway "$user_id" "BTCUSDT" "BUY" "LIMIT" "$price" "$qty" "$client_id")
    local order_id
    order_id=$(parse_order_id "$resp")
    assert_non_empty "$order_id" "Cancel order id missing"

    cancel_order gateway "$user_id" "BTCUSDT" "$order_id" >/dev/null

    local status
    status=$(check_order_status gateway "$user_id" "$order_id")
    if [ "$status" != "4" ] && [ "$status" != "CANCELED" ]; then
        log_error "Unexpected cancel status: $status"
        return 1
    fi

    local after_frozen
    after_frozen=$(check_balance gateway "$user_id" "$asset" "Frozen")
    if [ -n "$before_frozen" ] && [ -n "$after_frozen" ]; then
        if [ "$after_frozen" -gt "$before_frozen" ]; then
            log_error "Frozen balance not released after cancel"
            return 1
        fi
    fi
}

test_websocket() {
    log_info "Test: WebSocket private events"

    local tmpfile
    tmpfile=$(mktemp)
    local listener_pid
    listener_pid=$(start_ws_listener "$tmpfile" 10)

    local price="32000"
    local qty="0.1"
    local ts
    ts=$(now_ms)

    deposit "$WS_USER_ID" "BTC" "$(scale_asset_amount "BTC" "1")"

    place_order direct "$WS_USER_ID" "BTCUSDT" "SELL" "LIMIT" "$price" "$qty" "e2e-ws-sell-${ts}" >/dev/null
    place_order gateway "$WS_USER_ID" "BTCUSDT" "BUY" "LIMIT" "$price" "$qty" "e2e-ws-buy-${ts}" >/dev/null

    sleep 2

    if kill -0 "$listener_pid" >/dev/null 2>&1; then
        wait "$listener_pid" || true
    fi

    if ! grep -q '"channel":"order"' "$tmpfile"; then
        log_error "No order event received on WebSocket"
        return 1
    fi
    if ! grep -q '"channel":"trade"' "$tmpfile"; then
        log_error "No trade event received on WebSocket"
        return 1
    fi
    if ! grep -q '"channel":"balance"' "$tmpfile"; then
        log_error "No balance event received on WebSocket"
        return 1
    fi

    rm -f "$tmpfile"
}

test_price_protection() {
    log_info "Test: Price protection"

    local user_id=$WS_USER_ID
    local ts
    ts=$(now_ms)

    deposit "$user_id" "USDT" "$(scale_asset_amount "USDT" "5000")"
    deposit "$user_id" "BTC" "$(scale_asset_amount "BTC" "1")"

    local bid_price="30000"
    local ask_price="31000"
    local qty="0.1"

    local bid_resp
    bid_resp=$(place_order direct "$user_id" "BTCUSDT" "BUY" "LIMIT" "$bid_price" "$qty" "e2e-bid-${ts}")
    local bid_id
    bid_id=$(parse_order_id "$bid_resp")

    local ask_resp
    ask_resp=$(place_order direct "$user_id" "BTCUSDT" "SELL" "LIMIT" "$ask_price" "$qty" "e2e-ask-${ts}")
    local ask_id
    ask_id=$(parse_order_id "$ask_resp")

    local payload
    local out_price="50000"
    local out_price_int
    local qty_int
    out_price_int=$(scale_symbol_price "BTCUSDT" "$out_price")
    qty_int=$(scale_symbol_qty "BTCUSDT" "$qty")
    if [ -z "$out_price_int" ] || [ -z "$qty_int" ]; then
        log_error "Failed to scale price protection payload"
        return 1
    fi

    payload=$(cat <<JSON
{"symbol":"BTCUSDT","side":"BUY","type":"LIMIT","price":${out_price_int},"quantity":${qty_int},"clientOrderId":"e2e-out-${ts}"}
JSON
)

    local resp
    resp=$(curl -s -w "\n%{http_code}" -X POST "${ORDER_URL}/v1/order?userId=${user_id}" \
        -H "Content-Type: application/json" \
        -d "$payload")

    local body
    body=$(echo "$resp" | head -n 1)
    local code
    code=$(echo "$resp" | tail -n 1)

    if [ "$code" -ne 400 ]; then
        log_error "Expected HTTP 400 for price protection, got ${code}"
        return 1
    fi

    if ! echo "$body" | grep -q "PRICE_OUT_OF_RANGE"; then
        log_error "Expected PRICE_OUT_OF_RANGE, got: $body"
        return 1
    fi

    if [ -n "$bid_id" ]; then
        cancel_order direct "$user_id" "BTCUSDT" "$bid_id" >/dev/null || true
    fi
    if [ -n "$ask_id" ]; then
        cancel_order direct "$user_id" "BTCUSDT" "$ask_id" >/dev/null || true
    fi
}

test_reconciliation() {
    log_info "Test: Reconciliation"

    local output
    local clearing_dir="${SCRIPT_DIR}/../exchange-clearing"
    output=$(cd "$clearing_dir" && go run ./cmd/reconciliation/main.go --db-url "$DB_URL" 2>&1) || {
        log_error "Reconciliation failed: $output"
        return 1
    }
}

main() {
    log_info "=== Phase 1 E2E Tests ==="

    wait_for_services

    if [ "$AUTO_CLEANUP" -eq 1 ]; then
        cleanup_data
    fi

    if [ "$AUTO_CLEANUP" -eq 1 ]; then
        trap cleanup_data EXIT
    fi

    test_deposit_and_order && log_info "✓ Deposit & Order"
    test_matching && log_info "✓ Matching"
    test_cancel_order && log_info "✓ Cancel Order"
    test_websocket && log_info "✓ WebSocket"
    test_price_protection && log_info "✓ Price Protection"
    test_reconciliation && log_info "✓ Reconciliation"

    log_info "=== All Tests Passed ==="
}

main "$@"
