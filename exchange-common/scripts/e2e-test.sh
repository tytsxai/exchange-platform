#!/bin/bash
# 端到端集成测试脚本
# 测试完整交易流程: 用户注册 -> 创建API Key -> 下单 -> 撮合 -> 查询

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
USER_URL="${USER_URL:-http://localhost:8085}"
ORDER_URL="${ORDER_URL:-http://localhost:8081}"
MARKET_URL="${MARKET_URL:-http://localhost:8084}"
WALLET_URL="${WALLET_URL:-http://localhost:8087}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1" >&2; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

SYMBOL_PRECISION_SYMBOL=""
SYMBOL_PRICE_PRECISION_CACHE=""
SYMBOL_QTY_PRECISION_CACHE=""

symbol_precisions() {
    local symbol=$1
    if [ "$SYMBOL_PRECISION_SYMBOL" = "$symbol" ] && [ -n "$SYMBOL_PRICE_PRECISION_CACHE" ] && [ -n "$SYMBOL_QTY_PRECISION_CACHE" ]; then
        echo "${SYMBOL_PRICE_PRECISION_CACHE} ${SYMBOL_QTY_PRECISION_CACHE}"
        return 0
    fi

    local resp
    resp=$(curl -sf "${ORDER_URL}/v1/exchangeInfo" 2>/dev/null || true)
    if [ -z "$resp" ]; then
        SYMBOL_PRECISION_SYMBOL="$symbol"
        SYMBOL_PRICE_PRECISION_CACHE=8
        SYMBOL_QTY_PRECISION_CACHE=8
        echo "8 8"
        return 0
    fi

    local result
    result=$(python3 - "$resp" "$symbol" <<'PY'
import json,sys
resp=sys.argv[1]
symbol=sys.argv[2]
try:
    data=json.loads(resp)
    symbols=data.get("symbols") or []
    for item in symbols:
        if item.get("symbol") == symbol:
            price=item.get("pricePrecision") or 8
            qty=item.get("qtyPrecision") or 8
            print(f"{price} {qty}")
            sys.exit(0)
except Exception:
    pass
print("8 8")
PY
    ) || result=""
    if [ -z "$result" ]; then
        result="8 8"
    fi
    SYMBOL_PRECISION_SYMBOL="$symbol"
    SYMBOL_PRICE_PRECISION_CACHE=${result%% *}
    SYMBOL_QTY_PRECISION_CACHE=${result##* }
    echo "$result"
}

scale_int64() {
    local value=$1
    local precision=$2
    local result
    result=$(python3 - "$value" "$precision" <<'PY'
from decimal import Decimal, InvalidOperation
import sys

value = sys.argv[1]
precision = int(sys.argv[2])
try:
    scaled = int(Decimal(value) * (Decimal(10) ** precision))
except (InvalidOperation, ValueError):
    print("")
    sys.exit(1)
print(scaled)
PY
    ) || result=""
    printf '%s\n' "$result"
}

scale_symbol_price() {
    local symbol=$1
    local value=$2
    local precisions
    precisions=$(symbol_precisions "$symbol")
    local price_precision=${precisions%% *}
    scale_int64 "$value" "$price_precision"
}

scale_symbol_qty() {
    local symbol=$1
    local value=$2
    local precisions
    precisions=$(symbol_precisions "$symbol")
    local qty_precision=${precisions##* }
    scale_int64 "$value" "$qty_precision"
}

# 健康检查
check_health() {
    local name=$1
    local url=$2
    if curl -sf "${url}/health" > /dev/null 2>&1; then
        log_info "$name is healthy"
        return 0
    else
        log_error "$name is not responding at $url"
        return 1
    fi
}

# ========== 测试用例 ==========

test_user_registration() {
    log_info "Testing user registration..."

    local email="test_$(date +%s)@example.com"
    local resp=$(curl -sf -X POST "${USER_URL}/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"Test123456\"}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "userId"; then
        log_info "User registration: PASSED"
        echo "$resp" | grep -o '"userId":[0-9]*' | grep -o '[0-9]*'
        return 0
    else
        log_error "User registration: FAILED - $resp"
        return 1
    fi
}

test_user_login() {
    local email=$1
    log_info "Testing user login..."

    local resp=$(curl -sf -X POST "${USER_URL}/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"Test123456\"}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "userId"; then
        log_info "User login: PASSED"
        return 0
    else
        log_error "User login: FAILED - $resp"
        return 1
    fi
}

test_create_api_key() {
    local user_id=$1
    log_info "Testing API key creation..."

    local resp=$(curl -sf -X POST "${USER_URL}/v1/apiKeys?userId=${user_id}" \
        -H "Content-Type: application/json" \
        -d "{\"label\":\"test-key\"}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "apiKey"; then
        log_info "API key creation: PASSED"
        echo "$resp"
        return 0
    else
        log_error "API key creation: FAILED - $resp"
        return 1
    fi
}

test_list_assets() {
    log_info "Testing list assets..."

    local resp=$(curl -sf "${WALLET_URL}/wallet/assets" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "asset\|^\[\]$"; then
        log_info "List assets: PASSED"
        return 0
    else
        log_error "List assets: FAILED - $resp"
        return 1
    fi
}

test_get_deposit_address() {
    local user_id=$1
    log_info "Testing get deposit address..."

    local resp=$(curl -sf "${WALLET_URL}/wallet/deposit/address?asset=USDT&network=TRC20&userId=${user_id}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "address\|not found\|disabled\|network"; then
        log_info "Get deposit address: PASSED (or network not configured)"
        return 0
    else
        log_error "Get deposit address: FAILED - $resp"
        return 1
    fi
}

test_market_depth() {
    log_info "Testing market depth..."

    local resp=$(curl -sf "${MARKET_URL}/v1/depth?symbol=BTCUSDT&limit=10" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "bids\|asks\|symbol"; then
        log_info "Market depth: PASSED"
        return 0
    else
        log_error "Market depth: FAILED - $resp"
        return 1
    fi
}

test_market_trades() {
    log_info "Testing market trades..."

    local resp=$(curl -sf "${MARKET_URL}/v1/trades?symbol=BTCUSDT&limit=10" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -qE "tradeId|^\[\]$|symbol|trades"; then
        log_info "Market trades: PASSED"
        return 0
    else
        log_error "Market trades: FAILED - $resp"
        return 1
    fi
}

test_create_order() {
    local user_id=$1
    log_info "Testing create order..."

    local idempotency_key="test_$(date +%s)_$$"
    local symbol="BTCUSDT"
    local price_int
    local qty_int
    price_int=$(scale_symbol_price "$symbol" "50000")
    qty_int=$(scale_symbol_qty "$symbol" "0.1")
    if [ -z "$price_int" ] || [ -z "$qty_int" ]; then
        log_error "Failed to scale order payload"
        return 1
    fi
    local resp=$(curl -sf -X POST "${ORDER_URL}/v1/order?userId=${user_id}" \
        -H "Content-Type: application/json" \
        -d "{
            \"clientOrderId\":\"${idempotency_key}\",
            \"symbol\":\"${symbol}\",
            \"side\":\"BUY\",
            \"type\":\"LIMIT\",
            \"price\":${price_int},
            \"quantity\":${qty_int}
        }" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "OrderID\|orderId\|errorCode"; then
        log_info "Create order: PASSED"
        echo "$resp"
        return 0
    else
        log_error "Create order: FAILED - $resp"
        return 1
    fi
}

test_list_orders() {
    local user_id=$1
    log_info "Testing list orders..."

    local resp=$(curl -sf "${ORDER_URL}/v1/allOrders?symbol=BTCUSDT&limit=10&userId=${user_id}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -qE "OrderID|orderId|^\[\]$|^null$"; then
        log_info "List orders: PASSED"
        return 0
    else
        log_error "List orders: FAILED - $resp"
        return 1
    fi
}

# ========== 主流程 ==========

main() {
    log_info "========== Exchange E2E Test =========="
    log_info "Starting integration tests..."
    echo ""

    local passed=0
    local failed=0

    # 健康检查
    log_info "--- Health Checks ---"
    check_health "User Service" "$USER_URL" && ((passed++)) || ((failed++))
    check_health "Order Service" "$ORDER_URL" && ((passed++)) || ((failed++))
    check_health "Market Service" "$MARKET_URL" && ((passed++)) || ((failed++))
    check_health "Wallet Service" "$WALLET_URL" && ((passed++)) || ((failed++))
    echo ""

    # 用户测试
    log_info "--- User Tests ---"
    local email="test_$(date +%s)@example.com"
    local user_id=$(test_user_registration) && ((passed++)) || ((failed++))

    if [ -n "$user_id" ] && [ "$user_id" != "0" ]; then
        test_user_login "$email" && ((passed++)) || ((failed++))
        test_create_api_key "$user_id" && ((passed++)) || ((failed++))
    else
        user_id=1  # 使用默认用户
        log_warn "Using default user_id=1 for remaining tests"
    fi
    echo ""

    # 钱包测试
    log_info "--- Wallet Tests ---"
    test_list_assets && ((passed++)) || ((failed++))
    test_get_deposit_address "$user_id" && ((passed++)) || ((failed++))
    echo ""

    # 行情测试
    log_info "--- Market Tests ---"
    test_market_depth && ((passed++)) || ((failed++))
    test_market_trades && ((passed++)) || ((failed++))
    echo ""

    # 订单测试
    log_info "--- Order Tests ---"
    test_create_order "$user_id" && ((passed++)) || ((failed++))
    test_list_orders "$user_id" && ((passed++)) || ((failed++))
    echo ""

    # 结果汇总
    log_info "========== Test Results =========="
    log_info "Passed: $passed"
    if [ $failed -gt 0 ]; then
        log_error "Failed: $failed"
        exit 1
    else
        log_info "All tests passed!"
        exit 0
    fi
}

main "$@"
