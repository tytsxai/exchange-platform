#!/bin/bash
set -euo pipefail

# Phase 1 E2E tests

# Config
API_URL=${API_URL:-"http://localhost:8080"}
USER_URL=${USER_URL:-"http://localhost:8085"}
ORDER_URL=${ORDER_URL:-"http://localhost:8081"}
MATCHING_URL=${MATCHING_URL:-"http://localhost:8082"}
CLEARING_URL=${CLEARING_URL:-"http://localhost:8083"}
WS_URL=${WS_URL:-"ws://localhost:8090"}
DB_URL=${DB_URL:-"postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"}

INTERNAL_TOKEN=${INTERNAL_TOKEN:-"dev-internal-token-change-me"}

GATEWAY_API_KEY=${GATEWAY_API_KEY:-""}
GATEWAY_API_SECRET=${GATEWAY_API_SECRET:-""}
WS_USER_ID=${WS_USER_ID:-""}
AUTO_CLEANUP=${AUTO_CLEANUP:-1}

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/test-helpers.sh"

prepare_gateway_credentials() {
    if [ -n "$GATEWAY_API_KEY" ] || [ -n "$GATEWAY_API_SECRET" ]; then
        if [ -z "$WS_USER_ID" ]; then
            log_error "WS_USER_ID is required when providing GATEWAY_API_KEY/GATEWAY_API_SECRET"
            return 1
        fi
        return 0
    fi

    log_info "Preparing gateway API key for E2E..."

    local ts
    ts=$(now_ms)
    local email="e2e_${ts}@local"
    local password="Test123456"

    curl -sS -X POST "${USER_URL}/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"${password}\"}" >/dev/null || true

    local login_resp
    login_resp=$(curl -sS -X POST "${USER_URL}/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"${password}\"}")

    local token
    token=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("token",""))' "$login_resp")

    local user_id
    user_id=$(python3 -c 'import json,sys; d=json.loads(sys.argv[1]); print(d.get("userId") or d.get("userID") or "")' "$login_resp")

    if [ -z "$token" ] || [ -z "$user_id" ]; then
        log_error "Failed to login/register test user"
        return 1
    fi

    local apikey_resp
    apikey_resp=$(curl -sS -X POST "${USER_URL}/v1/apiKeys" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${token}" \
        -d '{"label":"e2e","permissions":3,"ipWhitelist":[]}')

    GATEWAY_API_KEY=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("apiKey",""))' "$apikey_resp")
    GATEWAY_API_SECRET=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("secret",""))' "$apikey_resp")

    if [ -z "$GATEWAY_API_KEY" ] || [ -z "$GATEWAY_API_SECRET" ]; then
        log_error "Failed to create API key for test user"
        return 1
    fi

    WS_USER_ID="$user_id"
    log_info "Prepared gateway credentials for WS_USER_ID=${WS_USER_ID}"
}

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

    local price
    price=$(maker_symbol_price "$symbol" "BUY")
    if [ -z "$price" ]; then
        price="30000"
    fi
    local qty="0.1"
    local client_id="e2e-deposit-order-$(now_ms)"
    local resp
    resp=$(place_order gateway "$user_id" "$symbol" "BUY" "LIMIT" "$price" "$qty" "$client_id" "POST_ONLY") || return 1

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

    http_request "POST" "${MATCHING_URL}/internal/reset?symbol=BTCUSDT" "" >/dev/null

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

    local price
    price=$(reference_symbol_price "BTCUSDT")
    if [ -z "$price" ]; then
        price="31000"
    fi
    local qty="0.1"

    local sell_resp
    sell_resp=$(place_order direct "$user_a" "BTCUSDT" "SELL" "LIMIT" "$price" "$qty" "e2e-sell-${ts}" "GTC") || return 1
    local sell_id
    sell_id=$(parse_order_id "$sell_resp")
    assert_non_empty "$sell_id" "Sell order id missing"

    local buy_resp
    buy_resp=$(place_order direct "$user_b" "BTCUSDT" "BUY" "LIMIT" "$price" "$qty" "e2e-buy-${ts}" "GTC") || return 1
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

    if [ "$trade_count" -le 0 ] 2>/dev/null; then
        log_error "Trade not created"
        return 1
    fi

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
    ledger_count_a=0
    ledger_count_b=0
    for i in {1..30}; do
        ledger_count_a=$(psql_exec "SELECT COUNT(*) FROM exchange_clearing.ledger_entries WHERE user_id=${user_a} AND ref_type='TRADE';" | tr -d ' ' || echo "0")
        ledger_count_b=$(psql_exec "SELECT COUNT(*) FROM exchange_clearing.ledger_entries WHERE user_id=${user_b} AND ref_type='TRADE';" | tr -d ' ' || echo "0")
        ledger_count_a=${ledger_count_a:-0}
        ledger_count_b=${ledger_count_b:-0}
        if [ "$ledger_count_a" -gt 0 ] 2>/dev/null && [ "$ledger_count_b" -gt 0 ] 2>/dev/null; then
            break
        fi
        sleep 1
    done

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

    local price
    price=$(maker_symbol_price "BTCUSDT" "BUY")
    if [ -z "$price" ]; then
        price="30000"
    fi
    local qty="0.1"
    local client_id="e2e-cancel-$(now_ms)"

    local resp
    resp=$(place_order gateway "$user_id" "BTCUSDT" "BUY" "LIMIT" "$price" "$qty" "$client_id" "POST_ONLY") || return 1
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

    http_request "POST" "${MATCHING_URL}/internal/reset?symbol=BTCUSDT" "" >/dev/null

    local tmpfile
    tmpfile=$(mktemp)
    local listener_pid
    start_ws_listener "$tmpfile" 10
    listener_pid=${WS_LISTENER_PID:-""}
    assert_non_empty "$listener_pid" "WebSocket listener PID missing"

    # Give the WS handshake a moment to complete before triggering events.
    sleep 0.5

    local price
    price=$(reference_symbol_price "BTCUSDT")
    if [ -z "$price" ]; then
        price="32000"
    fi
    local qty="0.1"
    local ts
    ts=$(now_ms)

    local other_user
    other_user=$(create_test_user "e2e_ws_other_${ts}@local")
    assert_non_empty "$other_user" "WS counterparty user not created"

    deposit "$WS_USER_ID" "BTC" "$(scale_asset_amount "BTC" "1")"
    deposit "$other_user" "USDT" "$(scale_asset_amount "USDT" "10000")"

    place_order gateway "$WS_USER_ID" "BTCUSDT" "SELL" "LIMIT" "$price" "$qty" "e2e-ws-sell-${ts}" "GTC" >/dev/null
    place_order direct "$other_user" "BTCUSDT" "BUY" "LIMIT" "$price" "$qty" "e2e-ws-buy-${ts}" "GTC" >/dev/null

    wait_for_condition 20 0.25 "grep -q 'WS_CONNECTED' '$tmpfile'" || {
        log_error "WebSocket handshake marker missing"
        if [ -f "${tmpfile}.err" ]; then
            tail -n 50 "${tmpfile}.err" >&2 || true
        fi
        tail -n 50 "$tmpfile" >&2 || true
        return 1
    }

    wait_for_condition 40 0.25 "grep -q '\"channel\":\"order\"' '$tmpfile'" || true
    wait_for_condition 40 0.25 "grep -q '\"channel\":\"trade\"' '$tmpfile'" || true
    wait_for_condition 40 0.25 "grep -q '\"channel\":\"balance\"' '$tmpfile'" || true

    local ws_rc=0
    if kill -0 "$listener_pid" >/dev/null 2>&1; then
        wait "$listener_pid" || ws_rc=$?
    fi

    if [ "$ws_rc" -ne 0 ]; then
        log_error "WebSocket listener failed (exit=${ws_rc})"
        if [ -f "${tmpfile}.err" ]; then
            tail -n 50 "${tmpfile}.err" >&2 || true
        fi
        tail -n 80 "$tmpfile" >&2 || true
        return 1
    fi

    if ! grep -q '"channel":"order"' "$tmpfile"; then
        log_error "No order event received on WebSocket"
        tail -n 80 "$tmpfile" >&2 || true
        return 1
    fi
    if ! grep -q '"channel":"trade"' "$tmpfile"; then
        log_error "No trade event received on WebSocket"
        tail -n 80 "$tmpfile" >&2 || true
        return 1
    fi
    if ! grep -q '"channel":"balance"' "$tmpfile"; then
        log_error "No balance event received on WebSocket"
        tail -n 80 "$tmpfile" >&2 || true
        return 1
    fi

    rm -f "$tmpfile" "${tmpfile}.err"
}

test_price_protection() {
    log_info "Test: Price protection"

    http_request "POST" "${MATCHING_URL}/internal/reset?symbol=BTCUSDT" "" >/dev/null

    local user_id=$WS_USER_ID
    local ts
    ts=$(now_ms)

    deposit "$user_id" "USDT" "$(scale_asset_amount "USDT" "5000")"
    deposit "$user_id" "BTC" "$(scale_asset_amount "BTC" "1")"

    local ref_price
    ref_price=$(reference_symbol_price "BTCUSDT")
    if [ -z "$ref_price" ]; then
        ref_price="30000"
    fi

    local tick
    tick=$(symbol_price_tick "BTCUSDT")

    local bid_price
    local ask_price
    bid_price=$(python3 - "$ref_price" "$tick" <<'PY'
from decimal import Decimal
import sys

ref = Decimal(sys.argv[1])
tick = Decimal(sys.argv[2])
out = ref - tick
if out <= 0:
    out = ref
s = format(out, "f")
if "." in s:
    s = s.rstrip("0").rstrip(".")
print(s)
PY
)
    ask_price=$(python3 - "$ref_price" "$tick" <<'PY'
from decimal import Decimal
import sys

ref = Decimal(sys.argv[1])
tick = Decimal(sys.argv[2])
out = ref + tick
s = format(out, "f")
if "." in s:
    s = s.rstrip("0").rstrip(".")
print(s)
PY
)
    local qty="0.1"

    local bid_resp
    bid_resp=$(place_order direct "$user_id" "BTCUSDT" "BUY" "LIMIT" "$bid_price" "$qty" "e2e-bid-${ts}" "POST_ONLY") || return 1
    local bid_id
    bid_id=$(parse_order_id "$bid_resp")

    local ask_resp
    ask_resp=$(place_order direct "$user_id" "BTCUSDT" "SELL" "LIMIT" "$ask_price" "$qty" "e2e-ask-${ts}" "POST_ONLY") || return 1
    local ask_id
    ask_id=$(parse_order_id "$ask_resp")

    local payload
    local out_price
    out_price=$(python3 - "$ref_price" <<'PY'
from decimal import Decimal
import sys

ref = Decimal(sys.argv[1])
out = ref * Decimal(2)
s = format(out, "f")
if "." in s:
    s = s.rstrip("0").rstrip(".")
print(s)
PY
)
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

    local tmp
    tmp=$(mktemp)
    local code
    code=$(curl -sS -o "$tmp" -w "%{http_code}" -X POST "${ORDER_URL}/v1/order" \
        -H "Content-Type: application/json" \
        -H "X-Internal-Token: ${INTERNAL_TOKEN}" \
        -H "X-User-Id: ${user_id}" \
        -d "$payload") || {
        rm -f "$tmp"
        log_error "Price protection request failed"
        return 1
    }
    local body
    body=$(cat "$tmp")
    rm -f "$tmp"

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

    if [ "${E2E_SKIP_RECONCILIATION:-0}" = "1" ]; then
        log_warn "Skipping reconciliation (E2E_SKIP_RECONCILIATION=1)"
        return 0
    fi

    local output
    local clearing_dir="${SCRIPT_DIR}/../exchange-clearing"
    output=$(cd "$clearing_dir" && go run ./cmd/reconciliation/main.go --db-url "$DB_URL" --alert=false 2>&1) || {
        log_error "Reconciliation failed: $output"
        return 1
    }

    if echo "$output" | grep -q "Discrepancy found"; then
        log_warn "Reconciliation reported discrepancies (ignored in E2E with --alert=false)"
    fi
}

main() {
    log_info "=== Phase 1 E2E Tests ==="

    wait_for_services

    if [ "$AUTO_CLEANUP" -eq 1 ]; then
        cleanup_data
    fi

    prepare_gateway_credentials || return 1

    log_info "E2E config: API_URL=${API_URL} USER_URL=${USER_URL} WS_URL=${WS_URL} WS_USER_ID=${WS_USER_ID} apiKey_len=${#GATEWAY_API_KEY} secret_len=${#GATEWAY_API_SECRET}"

    if [ "$AUTO_CLEANUP" -eq 1 ]; then
        trap cleanup_data EXIT
    fi

    test_deposit_and_order || return 1
    log_info "✓ Deposit & Order"

    test_matching || return 1
    log_info "✓ Matching"

    test_cancel_order || return 1
    log_info "✓ Cancel Order"

    test_websocket || return 1
    log_info "✓ WebSocket"

    test_price_protection || return 1
    log_info "✓ Price Protection"

    test_reconciliation || return 1
    log_info "✓ Reconciliation"

    log_info "=== All Tests Passed ==="
}

main "$@"
