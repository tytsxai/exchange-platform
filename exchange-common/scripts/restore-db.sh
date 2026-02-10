#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-""}
BACKUP_FILE=${1:-}
APP_ENV=${APP_ENV:-"dev"}

if [ "$APP_ENV" != "dev" ] && [ -z "$DB_URL" ]; then
  echo "In non-dev environment, DB_URL is required for restore-db.sh" >&2
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

if [ -z "$DB_URL" ]; then
  DB_HOST=${DB_HOST:-"localhost"}
  DB_PORT=${DB_PORT:-"5436"}
  DB_USER=${DB_USER:-"exchange"}
  DB_PASSWORD=${DB_PASSWORD:-"exchange123"}
  DB_NAME=${DB_NAME:-"exchange"}
  DB_SSL_MODE=${DB_SSL_MODE:-"disable"}
  DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL_MODE}"
fi

if [ -z "$BACKUP_FILE" ]; then
  echo "Usage: $0 <backup-file>" >&2
  exit 1
fi

if [ ! -f "$BACKUP_FILE" ]; then
  echo "Backup file not found: $BACKUP_FILE" >&2
  exit 1
fi

if ! command -v pg_restore >/dev/null 2>&1; then
  echo "pg_restore not found in PATH" >&2
  exit 1
fi

pg_restore --clean --if-exists -d "$DB_URL" "$BACKUP_FILE"
echo "Restore completed from ${BACKUP_FILE}"
