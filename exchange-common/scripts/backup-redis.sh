#!/bin/bash
set -euo pipefail

REDIS_ADDR=${REDIS_ADDR:-"localhost:6380"}
OUT_DIR=${OUT_DIR:-"./backups"}
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
FILE="${OUT_DIR}/redis_${TIMESTAMP}.rdb"

HOST=${REDIS_ADDR%%:*}
PORT=${REDIS_ADDR##*:}

mkdir -p "$OUT_DIR"

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli not found in PATH" >&2
  exit 1
fi

redis-cli -h "$HOST" -p "$PORT" --rdb "$FILE"
echo "Redis snapshot saved to ${FILE}"
