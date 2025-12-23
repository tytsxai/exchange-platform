#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-"postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"}
OUT_DIR=${OUT_DIR:-"./backups"}
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
FILE="${OUT_DIR}/exchange_${TIMESTAMP}.dump"

mkdir -p "$OUT_DIR"

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "pg_dump not found in PATH" >&2
  exit 1
fi

pg_dump -Fc "$DB_URL" -f "$FILE"
echo "Backup saved to ${FILE}"
