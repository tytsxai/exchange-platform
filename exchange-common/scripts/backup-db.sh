#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-""}
OUT_DIR=${OUT_DIR:-"./backups"}
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
FILE="${OUT_DIR}/exchange_${TIMESTAMP}.dump"

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
echo "Backup saved to ${FILE}"
