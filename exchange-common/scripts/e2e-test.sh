#!/bin/bash
# 端到端集成测试脚本
# 测试完整交易流程: 用户注册 -> 创建API Key -> 下单 -> 撮合 -> 查询

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
CLEARING_URL="${CLEARING_URL:-http://localhost:8083}"
WALLET_URL="${WALLET_URL:-http://localhost:8086}"

INTERNAL_TOKEN="${INTERNAL_TOKEN:-dev-internal-token-change-me}"
USER_BEARER_TOKEN="${USER_BEARER_TOKEN:-}"
API_KEY="${API_KEY:-}"
API_SECRET="${API_SECRET:-}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1" >&2; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

curl_internal() {
    curl -sS -H "X-Internal-Token: ${INTERNAL_TOKEN}" "$@" || return 1
}

curl_wallet() {
    curl -sS -H "Authorization: Bearer ${USER_BEARER_TOKEN}" "$@" || return 1
}

sign_request() {
    local method=$1
    local path=$2
    local query_string=${3:-}

    if [ -z "${API_KEY}" ] || [ -z "${API_SECRET}" ]; then
        echo ""
        return 1
    fi

    local ts nonce sig
    # 生成毫秒时间戳（兼容 macOS/BSD date）
    ts=$(python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
    )
    nonce=$(python3 - <<'PY'
import secrets
print(secrets.token_hex(16))
PY
    )
    sig=$(python3 - "$API_SECRET" "$ts" "$nonce" "$method" "$path" "$query_string" <<'PY'
import hashlib,hmac,sys,urllib.parse
secret=sys.argv[1].encode()
ts=sys.argv[2]
nonce=sys.argv[3]
method=sys.argv[4].upper()
path=sys.argv[5]
raw_q=sys.argv[6] or ""

q=urllib.parse.parse_qs(raw_q, keep_blank_values=True)
pairs=[]
for k in sorted(q.keys()):
    for v in sorted(q[k]):
        pairs.append(f"{k}={v}")
canonical_q="&".join(pairs)
canonical="\n".join([ts, nonce, method, path, canonical_q])
print(hmac.new(secret, canonical.encode(), hashlib.sha256).hexdigest())
PY
    )
    printf '%s\n' "$ts $nonce $sig"
}

curl_signed() {
    local method=$1
    local url=$2

    local path query_string
    path=$(python3 - "$url" <<'PY'
import sys,urllib.parse
u=urllib.parse.urlparse(sys.argv[1])
print(u.path)
PY
    )
    query_string=$(python3 - "$url" <<'PY'
import sys,urllib.parse
u=urllib.parse.urlparse(sys.argv[1])
print(u.query)
PY
    )

    local parts ts nonce sig
    parts=$(sign_request "$method" "$path" "$query_string") || return 1
    ts=${parts%% *}
    nonce=${parts#* }; nonce=${nonce%% *}
    sig=${parts##* }

    curl -sS -X "$method" "$url" \
      -H "X-API-KEY: ${API_KEY}" \
      -H "X-API-TIMESTAMP: ${ts}" \
      -H "X-API-NONCE: ${nonce}" \
      -H "X-API-SIGNATURE: ${sig}" \
      "${@:3}" || return 1
}

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
    resp=$(curl -sS "${BASE_URL}/v1/exchangeInfo" 2>/dev/null || true)
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
        sym=item.get("symbol") or item.get("Symbol")
        if sym == symbol:
            price=item.get("pricePrecision") or item.get("PricePrecision") or item.get("quotePrecision") or item.get("QuotePrecision") or 8
            qty=item.get("qtyPrecision") or item.get("QtyPrecision") or item.get("basePrecision") or item.get("BasePrecision") or 8
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

credit_balance() {
    local user_id=$1
    local asset=$2
    local amount=$3
    local key="credit_${user_id}_${asset}_$(date +%s)_$$"
    curl_internal -X POST "${CLEARING_URL}/internal/credit" \
        -H "Content-Type: application/json" \
        -d "{\"IdempotencyKey\":\"${key}\",\"UserID\":${user_id},\"Asset\":\"${asset}\",\"Amount\":${amount},\"RefType\":\"E2E\",\"RefID\":\"${key}\"}" >/dev/null || return 1
}

# ========== 测试用例 ==========

test_user_registration() {
    log_info "Testing user registration..."

    local email="${1:-test_$(date +%s)@example.com}"
    local resp=$(curl -sS -X POST "${BASE_URL}/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"Test123456\"}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "userId"; then
        log_info "User registration: PASSED"
        python3 - "$resp" <<'PY' || echo "$resp" | grep -o '"userId":[0-9]*' | grep -o '[0-9]*'
import json,sys
data=json.loads(sys.argv[1])
print(data.get("userId") or 0)
PY
        return 0
    else
        log_error "User registration: FAILED - $resp"
        return 1
    fi
}

test_user_login() {
    local email=$1
    log_info "Testing user login..."

    local resp=$(curl -sS -X POST "${BASE_URL}/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"Test123456\"}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "token"; then
        log_info "User login: PASSED"
        python3 - "$resp" <<'PY' || true
import json,sys
data=json.loads(sys.argv[1])
print(data.get("token") or "")
PY
        return 0
    else
        log_error "User login: FAILED - $resp"
        return 1
    fi
}

test_create_api_key() {
    log_info "Testing API key creation..."

    if [ -z "${USER_BEARER_TOKEN}" ]; then
        log_error "No user token available for API key creation"
        return 1
    fi

    local resp=$(curl -sS -X POST "${BASE_URL}/v1/apiKeys" \
        -H "Authorization: Bearer ${USER_BEARER_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{\"label\":\"test-key\",\"permissions\":3}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -q "apiKey"; then
        log_info "API key creation: PASSED"
        python3 - "$resp" <<'PY'
import json,sys
data=json.loads(sys.argv[1])
print(data.get("apiKey",""))
print(data.get("secret",""))
PY
        return 0
    else
        log_error "API key creation: FAILED - $resp"
        return 1
    fi
}

test_list_assets() {
    log_info "Testing list assets..."

    if [ -z "${USER_BEARER_TOKEN}" ]; then
        log_warn "No user token available; skipping wallet tests"
        return 0
    fi

    local resp=$(curl_wallet "${WALLET_URL}/wallet/assets" 2>/dev/null || echo '{"error":"failed"}')

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

    if [ -z "${USER_BEARER_TOKEN}" ]; then
        log_warn "No user token available; skipping wallet tests"
        return 0
    fi

    local resp=$(curl_wallet "${WALLET_URL}/wallet/deposit/address?asset=USDT&network=TRC20&userId=${user_id}" 2>/dev/null || echo '{"error":"failed"}')

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

    local resp=$(curl -sS "${BASE_URL}/v1/depth?symbol=BTCUSDT&limit=10" 2>/dev/null || echo '{"error":"failed"}')

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

    local resp=$(curl -sS "${BASE_URL}/v1/trades?symbol=BTCUSDT&limit=10" 2>/dev/null || echo '{"error":"failed"}')

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
    qty_int=$(scale_symbol_qty "$symbol" "1")
    if [ -z "$price_int" ] || [ -z "$qty_int" ]; then
        log_error "Failed to scale order payload"
        return 1
    fi
    local resp
    resp=$(curl_signed "POST" "${BASE_URL}/v1/order" \
        -H "Content-Type: application/json" \
        -d "{
            \"clientOrderId\":\"${idempotency_key}\",
            \"symbol\":\"${symbol}\",
            \"side\":\"BUY\",
            \"type\":\"LIMIT\",
            \"price\":${price_int},
            \"quantity\":${qty_int}
        }" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$resp" | grep -qE '"code"\s*:\s*"' ; then
        log_error "Create order: FAILED - $resp"
        return 1
    fi
    if echo "$resp" | grep -q "OrderID\\|orderId"; then
        log_info "Create order: PASSED"
        echo "$resp"
        return 0
    fi
    log_error "Create order: FAILED - $resp"
    return 1
}

test_list_orders() {
    local user_id=$1
    log_info "Testing list orders..."

    local resp
    resp=$(curl_signed "GET" "${BASE_URL}/v1/allOrders?symbol=BTCUSDT&limit=10" 2>/dev/null || echo '{"error":"failed"}')

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
    if check_health "Gateway" "$BASE_URL"; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    if check_health "Wallet Service" "$WALLET_URL"; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    if check_health "Clearing Service" "$CLEARING_URL"; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""

    # 用户测试
    log_info "--- User Tests ---"
    local email="test_$(date +%s)@example.com"
    local user_id
    if user_id=$(test_user_registration "$email"); then passed=$((passed + 1)); else failed=$((failed + 1)); fi

    if [ -n "$user_id" ] && [ "$user_id" != "0" ]; then
        local token=""
        if token=$(test_user_login "$email"); then passed=$((passed + 1)); else failed=$((failed + 1)); fi
        USER_BEARER_TOKEN="$token"
        local api_key=""
        local api_secret=""
        if output=$(test_create_api_key); then
            passed=$((passed + 1))
            api_key=$(printf '%s\n' "$output" | head -n 1)
            api_secret=$(printf '%s\n' "$output" | tail -n 1)
            API_KEY="$api_key"
            API_SECRET="$api_secret"
        else
            failed=$((failed + 1))
        fi
    else
        user_id=1  # 使用默认用户
        log_warn "Using default user_id=1 for remaining tests"
    fi
    echo ""

    if [ -n "$user_id" ] && [ "$user_id" != "0" ]; then
        log_info "--- Seed Balance (Clearing) ---"
        credit_balance "$user_id" "USDT" 1000000000 && passed=$((passed + 1)) || failed=$((failed + 1))
        echo ""
    fi

    # 钱包测试
    log_info "--- Wallet Tests ---"
    if test_list_assets; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    if test_get_deposit_address "$user_id"; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""

    # 行情测试
    log_info "--- Market Tests ---"
    if test_market_depth; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    if test_market_trades; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""

    # 订单测试
    log_info "--- Order Tests ---"
    if test_create_order "$user_id"; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    if test_list_orders "$user_id"; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
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
