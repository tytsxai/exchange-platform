#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-""}
OUT_DIR=${OUT_DIR:-"./backups"}
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
FILE="${OUT_DIR}/exchange_${TIMESTAMP}.dump"
APP_ENV=${APP_ENV:-"dev"}
KEEP_DAYS=${KEEP_DAYS:-30}

if [ "$APP_ENV" != "dev" ] && [ -z "$DB_URL" ]; then
  echo "In non-dev environment, DB_URL is required for backup-db.sh" >&2
  exit 1
fi

if [ "$APP_ENV" != "dev" ] && [ -n "$DB_URL" ]; then
  if echo "$DB_URL" | grep -Eq 'sslmode=disable([&#]|$)'; then
    echo "In non-dev environment, DB_URL must not use sslmode=disable" >&2
    exit 1
  fi
  if echo "$DB_URL" | grep -Eq 'exchange123'; then
    echo "In non-dev environment, DB_URL must not use default password" >&2
    exit 1
  fi
fi

if ! [[ "$KEEP_DAYS" =~ ^[0-9]+$ ]]; then
  echo "KEEP_DAYS must be a non-negative integer" >&2
  exit 1
fi

if [ -z "$DB_URL" ]; then
  DB_HOST=${DB_HOST:-"localhost"}
  DB_PORT=${DB_PORT:-"5436"}
  DB_USER=${DB_USER:-"exchange"}
  DB_PASSWORD=${DB_PASSWORD:-"exchange123"}
  DB_NAME=${DB_NAME:-"exchange"}
  DB_SSL_MODE=${DB_SSL_MODE:-"disable"}
  DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL_MODE}"
fi

mkdir -p "$OUT_DIR"

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "pg_dump not found in PATH" >&2
  exit 1
fi

pg_dump -Fc "$DB_URL" -f "$FILE"
if [ ! -s "$FILE" ]; then
  echo "Backup file is empty: ${FILE}" >&2
  exit 1
fi
echo "Backup saved to ${FILE}"

if [ "$KEEP_DAYS" -gt 0 ]; then
  old_files=$(find "$OUT_DIR" -maxdepth 1 -type f -name 'exchange_*.dump' -mtime +"$KEEP_DAYS" -print)
  if [ -n "$old_files" ]; then
    echo "Pruning backups older than ${KEEP_DAYS} days:"
    echo "$old_files"
    find "$OUT_DIR" -maxdepth 1 -type f -name 'exchange_*.dump' -mtime +"$KEEP_DAYS" -delete
  fi
fi
