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

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { printf '%b\n' "${GREEN}[INFO]${NC} $1" >&2; }
log_warn() { printf '%b\n' "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { printf '%b\n' "${RED}[ERROR]${NC} $1" >&2; }

check() {
  local name=$1
  local url=$2
  if curl -fsS "$url" >/dev/null; then
    log_info "${name} OK -> ${url}"
  else
    log_error "${name} FAIL -> ${url}"
    return 1
  fi
}

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

if [ "${RUN_E2E}" = "1" ]; then
  log_warn "RUN_E2E=1 enabled; executing scripts/e2e-test.sh"
  bash scripts/e2e-test.sh
fi

log_info "Production readiness checks finished."
