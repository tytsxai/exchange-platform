#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-"postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"}
BACKUP_FILE=${1:-}

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
