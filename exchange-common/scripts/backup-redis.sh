#!/bin/bash
set -euo pipefail

REDIS_ADDR=${REDIS_ADDR:-"localhost:6380"}
REDIS_USERNAME=${REDIS_USERNAME:-""}
REDIS_PASSWORD=${REDIS_PASSWORD:-""}
REDIS_TLS=${REDIS_TLS:-"false"}
REDIS_CACERT=${REDIS_CACERT:-""}
REDIS_CERT=${REDIS_CERT:-""}
REDIS_KEY=${REDIS_KEY:-""}
OUT_DIR=${OUT_DIR:-"./backups"}
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
FILE="${OUT_DIR}/redis_${TIMESTAMP}.rdb"
APP_ENV=${APP_ENV:-"dev"}
KEEP_DAYS=${KEEP_DAYS:-30}

if [ "$APP_ENV" != "dev" ] && [ -z "$REDIS_PASSWORD" ]; then
  echo "In non-dev environment, REDIS_PASSWORD is required for backup-redis.sh" >&2
  exit 1
fi

if ! [[ "$KEEP_DAYS" =~ ^[0-9]+$ ]]; then
  echo "KEEP_DAYS must be a non-negative integer" >&2
  exit 1
fi

HOST=${REDIS_ADDR%%:*}
PORT=${REDIS_ADDR##*:}

mkdir -p "$OUT_DIR"

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli not found in PATH" >&2
  exit 1
fi

args=()
if [ -n "$REDIS_USERNAME" ]; then
  args+=("--user" "$REDIS_USERNAME")
fi
if [ "$REDIS_TLS" = "true" ] || [ "$REDIS_TLS" = "1" ]; then
  args+=("--tls")
fi
if [ -n "$REDIS_CACERT" ]; then
  args+=("--cacert" "$REDIS_CACERT")
fi
if [ -n "$REDIS_CERT" ]; then
  args+=("--cert" "$REDIS_CERT")
fi
if [ -n "$REDIS_KEY" ]; then
  args+=("--key" "$REDIS_KEY")
fi

if [ -n "$REDIS_PASSWORD" ]; then
  export REDISCLI_AUTH="$REDIS_PASSWORD"
fi

redis-cli -h "$HOST" -p "$PORT" "${args[@]}" --rdb "$FILE"
if [ ! -s "$FILE" ]; then
  echo "Redis snapshot file is empty: ${FILE}" >&2
  exit 1
fi
echo "Redis snapshot saved to ${FILE}"

if [ "$KEEP_DAYS" -gt 0 ]; then
  old_files=$(find "$OUT_DIR" -maxdepth 1 -type f -name 'redis_*.rdb' -mtime +"$KEEP_DAYS" -print)
  if [ -n "$old_files" ]; then
    echo "Pruning Redis snapshots older than ${KEEP_DAYS} days:"
    echo "$old_files"
    find "$OUT_DIR" -maxdepth 1 -type f -name 'redis_*.rdb' -mtime +"$KEEP_DAYS" -delete
  fi
fi
