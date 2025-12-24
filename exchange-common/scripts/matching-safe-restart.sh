#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-prepare}"

ADMIN_BASE_URL="${ADMIN_BASE_URL:-http://localhost:8087}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
ADMIN_BEARER="${ADMIN_BEARER:-}"

REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"

ORDER_STREAM="${ORDER_STREAM:-exchange:orders}"
EVENT_STREAM="${EVENT_STREAM:-exchange:events}"

MATCHING_GROUP="${MATCHING_GROUP:-matching-group}"
ORDER_UPDATER_GROUP="${ORDER_UPDATER_GROUP:-order-updater-group}"
CLEARING_GROUP="${CLEARING_GROUP:-clearing-group}"
MARKETDATA_GROUP="${MARKETDATA_GROUP:-marketdata-group}"

DRAIN_TIMEOUT_SECS="${DRAIN_TIMEOUT_SECS:-60}"
SLEEP_SECS="${SLEEP_SECS:-2}"

require_cmd() {
  local cmd=$1
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

require_env() {
  local name=$1
  if [ -z "${!name:-}" ]; then
    echo "Missing required env: $name" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd redis-cli
require_env ADMIN_TOKEN
require_env ADMIN_BEARER

REDIS_HOST="${REDIS_ADDR%:*}"
REDIS_PORT="${REDIS_ADDR##*:}"
if [ "$REDIS_HOST" = "$REDIS_PORT" ]; then
  REDIS_HOST="$REDIS_ADDR"
  REDIS_PORT="6379"
fi

redis_cmd() {
  if [ -n "$REDIS_PASSWORD" ]; then
    redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" --raw "$@"
  else
    redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --raw "$@"
  fi
}

kill_switch() {
  local action=$1
  curl -sS -X POST "${ADMIN_BASE_URL}/admin/killSwitch" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${ADMIN_BEARER}" \
    -H "X-Admin-Token: ${ADMIN_TOKEN}" \
    -d "{\"action\":\"${action}\"}" >/dev/null
}

pending_count() {
  local stream=$1
  local group=$2
  local count
  count="$(redis_cmd XPENDING "$stream" "$group" 2>/dev/null | head -n1 || true)"
  if [ -z "$count" ]; then
    echo "0"
  else
    echo "$count"
  fi
}

group_lag() {
  local stream=$1
  local group=$2
  redis_cmd XINFO GROUPS "$stream" 2>/dev/null | awk -v group="$group" '
    $0=="name"{getline name; found=(name==group)}
    found && $0=="lag"{getline lag; print lag; exit}
  ' || true
}

wait_for_drain() {
  local start_ts elapsed
  start_ts=$(date +%s)
  while true; do
    local match_pending order_pending clearing_pending market_pending
    local match_lag order_lag clearing_lag market_lag

    match_pending=$(pending_count "$ORDER_STREAM" "$MATCHING_GROUP")
    order_pending=$(pending_count "$EVENT_STREAM" "$ORDER_UPDATER_GROUP")
    clearing_pending=$(pending_count "$EVENT_STREAM" "$CLEARING_GROUP")
    market_pending=$(pending_count "$EVENT_STREAM" "$MARKETDATA_GROUP")

    match_lag=$(group_lag "$ORDER_STREAM" "$MATCHING_GROUP")
    order_lag=$(group_lag "$EVENT_STREAM" "$ORDER_UPDATER_GROUP")
    clearing_lag=$(group_lag "$EVENT_STREAM" "$CLEARING_GROUP")
    market_lag=$(group_lag "$EVENT_STREAM" "$MARKETDATA_GROUP")

    echo "pending: matching=${match_pending} order-updater=${order_pending} clearing=${clearing_pending} marketdata=${market_pending}"
    if [ -n "$match_lag" ] || [ -n "$order_lag" ] || [ -n "$clearing_lag" ] || [ -n "$market_lag" ]; then
      echo "lag:     matching=${match_lag:-?} order-updater=${order_lag:-?} clearing=${clearing_lag:-?} marketdata=${market_lag:-?}"
    fi

    if [ "$match_pending" = "0" ] && [ "$order_pending" = "0" ] && [ "$clearing_pending" = "0" ] && [ "$market_pending" = "0" ]; then
      if [ -z "$match_lag" ] || [ "$match_lag" = "0" ]; then
        if [ -z "$order_lag" ] || [ "$order_lag" = "0" ]; then
          if [ -z "$clearing_lag" ] || [ "$clearing_lag" = "0" ]; then
            if [ -z "$market_lag" ] || [ "$market_lag" = "0" ]; then
              echo "streams drained"
              return 0
            fi
          fi
        fi
      fi
    fi

    elapsed=$(( $(date +%s) - start_ts ))
    if [ "$elapsed" -ge "$DRAIN_TIMEOUT_SECS" ]; then
      echo "timeout waiting for stream drain after ${DRAIN_TIMEOUT_SECS}s" >&2
      return 1
    fi
    sleep "$SLEEP_SECS"
  done
}

case "$ACTION" in
  prepare)
    echo "Setting kill switch to cancelOnly..."
    kill_switch "cancelOnly"
    echo "Waiting for stream drain..."
    wait_for_drain
    echo "Ready for matching restart. After deploy, run: $0 resume"
    ;;
  resume)
    echo "Resuming trading..."
    kill_switch "resume"
    ;;
  status)
    wait_for_drain
    ;;
  *)
    echo "Usage: $0 [prepare|resume|status]" >&2
    exit 1
    ;;
esac
