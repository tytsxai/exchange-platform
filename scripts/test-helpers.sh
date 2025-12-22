#!/bin/bash

# Shared helpers for E2E tests.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { printf '%b\n' "${GREEN}[INFO]${NC} $1" >&2; }
log_warn() { printf '%b\n' "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { printf '%b\n' "${RED}[ERROR]${NC} $1" >&2; }

require_cmd() {
    local cmd=$1
    if ! command -v "$cmd" >/dev/null 2>&1; then
        log_error "Missing required command: $cmd"
        return 1
    fi
}

now_ms() {
    # macOS 不支持 %N，使用 python 获取毫秒时间戳
    python3 -c "import time; print(int(time.time() * 1000))"
}

wait_for_services() {
    local attempts=${1:-60}
    local sleep_seconds=${2:-1}

    log_info "Waiting for services to be ready..."

    for ((i=1; i<=attempts; i++)); do
        local ok=0

        if curl -sf "${API_URL}/v1/ping" >/dev/null 2>&1 && \
           curl -sf "${USER_URL}/health" >/dev/null 2>&1 && \
           curl -sf "${ORDER_URL}/health" >/dev/null 2>&1 && \
           curl -sf "${MATCHING_URL}/health" >/dev/null 2>&1 && \
           curl -sf "${CLEARING_URL}/health" >/dev/null 2>&1; then
            ok=1
        fi

        if [ "$ok" -eq 1 ]; then
            log_info "All services are ready."
            return 0
        fi

        sleep "$sleep_seconds"
    done

    log_error "Services not ready after ${attempts} attempts."
    return 1
}

psql_exec() {
    require_cmd psql || return 1
    psql -X "${DB_URL}" -v ON_ERROR_STOP=1 -q -t -A -c "$1"
}

declare -A SYMBOL_PRICE_PRECISION
declare -A SYMBOL_QTY_PRECISION
declare -A ASSET_PRECISION

symbol_precisions() {
    local symbol=$1
    if [ -n "${SYMBOL_PRICE_PRECISION[$symbol]:-}" ] && [ -n "${SYMBOL_QTY_PRECISION[$symbol]:-}" ]; then
        echo "${SYMBOL_PRICE_PRECISION[$symbol]} ${SYMBOL_QTY_PRECISION[$symbol]}"
        return 0
    fi

    local row
    row=$(psql_exec "SELECT price_precision, qty_precision FROM exchange_order.symbol_configs WHERE symbol = '${symbol}' LIMIT 1;" | tr -d ' ')
    local price_precision=${row%%|*}
    local qty_precision=${row##*|}
    if [ -z "$price_precision" ] || [ -z "$qty_precision" ]; then
        price_precision=8
        qty_precision=8
    fi
    SYMBOL_PRICE_PRECISION[$symbol]=$price_precision
    SYMBOL_QTY_PRECISION[$symbol]=$qty_precision
    echo "$price_precision $qty_precision"
}

asset_precision() {
    local asset=$1
    if [ -n "${ASSET_PRECISION[$asset]:-}" ]; then
        echo "${ASSET_PRECISION[$asset]}"
        return 0
    fi
    local precision
    precision=$(psql_exec "SELECT precision FROM exchange_wallet.assets WHERE asset = '${asset}' LIMIT 1;" | tr -d ' ')
    if [ -z "$precision" ]; then
        precision=8
    fi
    ASSET_PRECISION[$asset]=$precision
    echo "$precision"
}

scale_factor() {
    local precision=$1
    local factor=1
    for ((i=0; i<precision; i++)); do
        factor=$((factor * 10))
    done
    echo "$factor"
}

scale_int64() {
    local value=$1
    local precision=${2:-8}
    python3 - "$value" "$precision" <<'PY'
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

scale_asset_amount() {
    local asset=$1
    local value=$2
    local precision
    precision=$(asset_precision "$asset")
    scale_int64 "$value" "$precision"
}

quote_amount() {
    local symbol=$1
    local price=$2
    local qty=$3
    local precisions
    precisions=$(symbol_precisions "$symbol")
    local price_precision=${precisions%% *}
    local qty_precision=${precisions##* }
    python3 - "$price" "$qty" "$price_precision" "$qty_precision" <<'PY'
from decimal import Decimal, InvalidOperation
import sys

price = sys.argv[1]
qty = sys.argv[2]
price_precision = int(sys.argv[3])
qty_precision = int(sys.argv[4])
try:
    price_int = int(Decimal(price) * (Decimal(10) ** price_precision))
    qty_int = int(Decimal(qty) * (Decimal(10) ** qty_precision))
    quote = price_int * qty_int // (10 ** qty_precision)
except (InvalidOperation, ValueError):
    print("")
    sys.exit(1)
print(quote)
PY
}

create_test_user() {
    local email=$1
    local password=${2:-"Test123456"}

    local existing_id
    existing_id=$(psql_exec "SELECT user_id FROM exchange_user.users WHERE email = '${email}' LIMIT 1;" | tr -d ' ')
    if [ -n "$existing_id" ]; then
        echo "$existing_id"
        return 0
    fi

    local resp
    resp=$(curl -sf -X POST "${USER_URL}/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${email}\",\"password\":\"${password}\"}" 2>/dev/null) || true

    if [ -n "$resp" ]; then
        local user_id
        user_id=$(python3 - "$resp" <<'PY'
import json,sys
try:
    data=json.loads(sys.argv[1])
    print(data.get('userId') or data.get('userID') or '')
except Exception:
    print('')
PY
)
        if [ -n "$user_id" ]; then
            echo "$user_id"
            return 0
        fi
    fi

    log_error "Failed to create test user for ${email}."
    return 1
}

deposit() {
    local user_id=$1
    local asset=$2
    local amount=$3
    local now
    now=$(now_ms)
    local ledger_id
    ledger_id=$((RANDOM * 1000000 + (RANDOM % 1000000)))
    local idempotency_key="e2e-deposit-${user_id}-${asset}-${now}-${RANDOM}"

    require_cmd psql || return 1

    psql -X "${DB_URL}" -v ON_ERROR_STOP=1 <<SQL
BEGIN;
INSERT INTO exchange_clearing.account_balances (user_id, asset, available, frozen, version, updated_at_ms)
VALUES (${user_id}, '${asset}', 0, 0, 0, ${now})
ON CONFLICT (user_id, asset) DO NOTHING;
WITH updated AS (
    UPDATE exchange_clearing.account_balances
    SET available = available + ${amount},
        version = version + 1,
        updated_at_ms = ${now}
    WHERE user_id = ${user_id} AND asset = '${asset}'
    RETURNING available, frozen
)
INSERT INTO exchange_clearing.ledger_entries (
    ledger_id, idempotency_key, user_id, asset, available_delta, frozen_delta,
    available_after, frozen_after, reason, ref_type, ref_id, note, created_at_ms
)
SELECT ${ledger_id}, '${idempotency_key}', ${user_id}, '${asset}', ${amount}, 0,
       available, frozen, 5, 'DEPOSIT', '${idempotency_key}', 'e2e deposit', ${now}
FROM updated;
COMMIT;
SQL
}

canonical_signature() {
    local method=$1
    local path=$2
    local query=$3
    local timestamp=$4
    local nonce=$5
    local secret=$6

    python3 - "$method" "$path" "$query" "$timestamp" "$nonce" "$secret" <<'PY'
import sys,urllib.parse,hmac,hashlib
method, path, query, ts, nonce, secret = sys.argv[1:]
params = urllib.parse.parse_qs(query, keep_blank_values=True)
parts = []
for key in sorted(params):
    for value in sorted(params[key]):
        parts.append(f"{key}={value}")
canonical_query = "&".join(parts)
canonical = "\n".join([ts, nonce, method.upper(), path, canonical_query])
sig = hmac.new(secret.encode(), canonical.encode(), hashlib.sha256).hexdigest()
print(sig)
PY
}

gateway_request() {
    local method=$1
    local path=$2
    local query=$3
    local body=$4

    local timestamp
    timestamp=$(now_ms)
    local nonce
    nonce=$(date +%s%N)
    local sig
    sig=$(canonical_signature "$method" "$path" "$query" "$timestamp" "$nonce" "$GATEWAY_API_SECRET")

    local url="${API_URL}${path}"
    if [ -n "$query" ]; then
        url+="?${query}"
    fi

    if [ -n "$body" ]; then
        curl -sf -X "$method" "$url" \
            -H "Content-Type: application/json" \
            -H "X-API-KEY: ${GATEWAY_API_KEY}" \
            -H "X-API-TIMESTAMP: ${timestamp}" \
            -H "X-API-NONCE: ${nonce}" \
            -H "X-API-SIGNATURE: ${sig}" \
            -d "$body"
    else
        curl -sf -X "$method" "$url" \
            -H "X-API-KEY: ${GATEWAY_API_KEY}" \
            -H "X-API-TIMESTAMP: ${timestamp}" \
            -H "X-API-NONCE: ${nonce}" \
            -H "X-API-SIGNATURE: ${sig}"
    fi
}

place_order() {
    local mode=$1
    local user_id=$2
    local symbol=$3
    local side=$4
    local type=$5
    local price=$6
    local quantity=$7
    local client_order_id=$8
    local price_int
    local qty_int
    price_int=$(scale_symbol_price "$symbol" "$price")
    qty_int=$(scale_symbol_qty "$symbol" "$quantity")
    if [ -z "$price_int" ] || [ -z "$qty_int" ]; then
        log_error "Failed to scale price/quantity for ${symbol}"
        return 1
    fi

    local payload
    payload=$(cat <<JSON
{"symbol":"${symbol}","side":"${side}","type":"${type}","price":${price_int},"quantity":${qty_int},"clientOrderId":"${client_order_id}"}
JSON
)

    if [ "$mode" = "gateway" ]; then
        gateway_request "POST" "/v1/order" "userId=${user_id}" "$payload"
    else
        curl -sf -X POST "${ORDER_URL}/v1/order?userId=${user_id}" \
            -H "Content-Type: application/json" \
            -d "$payload"
    fi
}

cancel_order() {
    local mode=$1
    local user_id=$2
    local symbol=$3
    local order_id=$4

    if [ "$mode" = "gateway" ]; then
        gateway_request "DELETE" "/v1/order" "userId=${user_id}&symbol=${symbol}&orderId=${order_id}" ""
    else
        curl -sf -X DELETE "${ORDER_URL}/v1/order?userId=${user_id}&symbol=${symbol}&orderId=${order_id}"
    fi
}

check_balance() {
    local mode=$1
    local user_id=$2
    local asset=$3
    local field=$4

    local resp
    if [ "$mode" = "gateway" ]; then
        resp=$(gateway_request "GET" "/v1/account" "" "")
    else
        resp=$(curl -sf "${CLEARING_URL}/v1/account?userId=${user_id}")
    fi

    python3 - "$resp" "$asset" "$field" <<'PY'
import json,sys
resp=json.loads(sys.argv[1])
asset=sys.argv[2]
field=sys.argv[3]
balances=resp.get('balances') or []
for b in balances:
    if b.get('Asset') == asset or b.get('asset') == asset:
        val=b.get(field) or b.get(field.capitalize()) or b.get(field.lower())
        print(val if val is not None else "")
        sys.exit(0)
print("")
PY
}

check_order_status() {
    local mode=$1
    local user_id=$2
    local order_id=$3

    local resp
    if [ "$mode" = "gateway" ]; then
        resp=$(gateway_request "GET" "/v1/order" "userId=${user_id}&orderId=${order_id}" "")
    else
        resp=$(curl -sf "${ORDER_URL}/v1/order?userId=${user_id}&orderId=${order_id}")
    fi

    python3 - "$resp" <<'PY'
import json,sys
resp=json.loads(sys.argv[1])
for key in ("status","Status"):
    if key in resp:
        print(resp[key])
        sys.exit(0)
print("")
PY
}

wait_for_condition() {
    local attempts=$1
    local sleep_seconds=$2
    local cmd=$3

    for ((i=1; i<=attempts; i++)); do
        if eval "$cmd"; then
            return 0
        fi
        sleep "$sleep_seconds"
    done
    return 1
}

ws_auth_url() {
    local timestamp
    timestamp=$(now_ms)
    local nonce
    nonce=$(date +%s%N)
    local query="apiKey=${GATEWAY_API_KEY}&timestamp=${timestamp}&nonce=${nonce}"
    local sig
    sig=$(canonical_signature "GET" "/ws/private" "$query" "$timestamp" "$nonce" "$GATEWAY_API_SECRET")
    echo "${WS_URL}/ws/private?${query}&signature=${sig}"
}

start_ws_listener() {
    local output_file=$1
    local timeout_seconds=${2:-8}
    local ws_url
    ws_url=$(ws_auth_url)

    python3 - "$ws_url" "$timeout_seconds" >"$output_file" 2>/dev/null <<'PY' &
import base64
import os
import socket
import sys
import time
import urllib.parse

ws_url = sys.argv[1]
timeout = float(sys.argv[2])

parsed = urllib.parse.urlparse(ws_url)
host = parsed.hostname
port = parsed.port or 80
path = parsed.path or "/"
if parsed.query:
    path = f"{path}?{parsed.query}"

key = base64.b64encode(os.urandom(16)).decode()
request = (
    f"GET {path} HTTP/1.1\r\n"
    f"Host: {host}:{port}\r\n"
    "Upgrade: websocket\r\n"
    "Connection: Upgrade\r\n"
    f"Sec-WebSocket-Key: {key}\r\n"
    "Sec-WebSocket-Version: 13\r\n"
    "\r\n"
)

sock = socket.create_connection((host, port), timeout=5)
sock.sendall(request.encode())
resp = sock.recv(4096).decode(errors="ignore")
if " 101 " not in resp:
    sys.exit(1)

sock.settimeout(0.5)
end = time.time() + timeout

def recv_exact(n):
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            return buf
        buf += chunk
    return buf

while time.time() < end:
    try:
        header = recv_exact(2)
        if len(header) < 2:
            break
        b1, b2 = header[0], header[1]
        opcode = b1 & 0x0F
        length = b2 & 0x7F
        if length == 126:
            length = int.from_bytes(recv_exact(2), "big")
        elif length == 127:
            length = int.from_bytes(recv_exact(8), "big")
        payload = recv_exact(length)
        if opcode == 1:
            sys.stdout.write(payload.decode(errors="ignore") + "\n")
            sys.stdout.flush()
        elif opcode == 8:
            break
    except socket.timeout:
        continue
    except Exception:
        break

sock.close()
PY
    echo $!
}
