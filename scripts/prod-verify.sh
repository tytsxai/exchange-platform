#!/bin/bash
set -euo pipefail

# Production readiness smoke checks.
# This script should be executed from an environment that can reach the services.

API_URL=${API_URL:-"http://localhost:8080"}
USER_URL=${USER_URL:-"http://localhost:8085"}
ORDER_URL=${ORDER_URL:-"http://localhost:8081"}
MATCHING_URL=${MATCHING_URL:-"http://localhost:8082"}
CLEARING_URL=${CLEARING_URL:-"http://localhost:8083"}
MARKETDATA_URL=${MARKETDATA_URL:-"http://localhost:8084"}
ADMIN_URL=${ADMIN_URL:-"http://localhost:8087"}
WALLET_URL=${WALLET_URL:-"http://localhost:8086"}

RUN_E2E=${RUN_E2E:-0}
CHECK_METRICS=${CHECK_METRICS:-0}
METRICS_TOKEN=${METRICS_TOKEN:-""}

# Avoid hanging indefinitely when one endpoint is unreachable.
CURL_CONNECT_TIMEOUT=${CURL_CONNECT_TIMEOUT:-3}
CURL_MAX_TIME=${CURL_MAX_TIME:-8}
CURL_RETRY=${CURL_RETRY:-2}
CURL_RETRY_DELAY=${CURL_RETRY_DELAY:-1}

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { printf '%b\n' "${GREEN}[INFO]${NC} $1" >&2; }
log_warn() { printf '%b\n' "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { printf '%b\n' "${RED}[ERROR]${NC} $1" >&2; }

CURL_ARGS=(
  -fsS
  --connect-timeout "${CURL_CONNECT_TIMEOUT}"
  --max-time "${CURL_MAX_TIME}"
  --retry "${CURL_RETRY}"
  --retry-delay "${CURL_RETRY_DELAY}"
)

check() {
  local name=$1
  local url=$2
  shift 2
  if curl "${CURL_ARGS[@]}" "$@" "$url" >/dev/null; then
    log_info "${name} OK -> ${url}"
    return 0
  fi
  log_error "${name} FAIL -> ${url}"
  return 1
}

log_info "Running /live checks..."
check "gateway live" "${API_URL}/live"
check "user live" "${USER_URL}/live"
check "order live" "${ORDER_URL}/live"
check "matching live" "${MATCHING_URL}/live"
check "clearing live" "${CLEARING_URL}/live"
check "marketdata live" "${MARKETDATA_URL}/live"
check "admin live" "${ADMIN_URL}/live"
check "wallet live" "${WALLET_URL}/live"

log_info "Running /ready checks..."
check "gateway ready" "${API_URL}/ready"
check "user ready" "${USER_URL}/ready"
check "order ready" "${ORDER_URL}/ready"
check "matching ready" "${MATCHING_URL}/ready"
check "clearing ready" "${CLEARING_URL}/ready"
check "marketdata ready" "${MARKETDATA_URL}/ready"
check "admin ready" "${ADMIN_URL}/ready"
check "wallet ready" "${WALLET_URL}/ready"

log_info "Running /health check on gateway..."
check "gateway health" "${API_URL}/health"

if [ "${CHECK_METRICS}" = "1" ]; then
  log_info "Running /metrics checks..."
  metrics_headers=()
  if [ -n "${METRICS_TOKEN}" ]; then
    metrics_headers=(-H "Authorization: Bearer ${METRICS_TOKEN}")
  fi
  check "gateway metrics" "${API_URL}/metrics" "${metrics_headers[@]}"
  check "user metrics" "${USER_URL}/metrics" "${metrics_headers[@]}"
  check "order metrics" "${ORDER_URL}/metrics" "${metrics_headers[@]}"
  check "matching metrics" "${MATCHING_URL}/metrics" "${metrics_headers[@]}"
  check "clearing metrics" "${CLEARING_URL}/metrics" "${metrics_headers[@]}"
  check "marketdata metrics" "${MARKETDATA_URL}/metrics" "${metrics_headers[@]}"
  check "admin metrics" "${ADMIN_URL}/metrics" "${metrics_headers[@]}"
  check "wallet metrics" "${WALLET_URL}/metrics" "${metrics_headers[@]}"
fi

if [ "${RUN_E2E}" = "1" ]; then
  log_warn "RUN_E2E=1 enabled; executing scripts/e2e-test.sh"
  bash scripts/e2e-test.sh
fi

log_info "Production readiness checks finished."
