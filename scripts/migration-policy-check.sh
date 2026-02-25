#!/bin/bash
set -euo pipefail

# Validate migration strategy for non-dev releases.
#
# Rules:
# - RUN_MIGRATIONS=true  -> DB_URL must be set.
# - RUN_MIGRATIONS=auto  -> pass when DB_URL is set; otherwise requires explicit skip ACK.
# - RUN_MIGRATIONS=false -> requires explicit skip ACK.
# - Any skip in non-dev requires MIGRATION_CHANGE_ID for audit traceability.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"
CLI_APP_ENV="${APP_ENV-}"
CLI_RUN_MIGRATIONS="${RUN_MIGRATIONS-}"
CLI_DB_URL="${DB_URL-}"
CLI_MIGRATIONS_SKIP_ACK="${MIGRATIONS_SKIP_ACK-}"
CLI_MIGRATION_CHANGE_ID="${MIGRATION_CHANGE_ID-}"

ENV_FILE_PATH="$PROD_ENV_FILE"
if [ ! -f "$ENV_FILE_PATH" ]; then
  ENV_FILE_PATH="$ROOT_DIR/$PROD_ENV_FILE"
fi
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "[migration-policy] env file not found: $PROD_ENV_FILE" >&2
  exit 1
fi

set -a
. "$ENV_FILE_PATH"
set +a

if [ -n "$CLI_APP_ENV" ]; then
  APP_ENV="$CLI_APP_ENV"
fi
if [ -n "$CLI_RUN_MIGRATIONS" ]; then
  RUN_MIGRATIONS="$CLI_RUN_MIGRATIONS"
fi
if [ -n "$CLI_DB_URL" ]; then
  DB_URL="$CLI_DB_URL"
fi
if [ -n "$CLI_MIGRATIONS_SKIP_ACK" ]; then
  MIGRATIONS_SKIP_ACK="$CLI_MIGRATIONS_SKIP_ACK"
fi
if [ -n "$CLI_MIGRATION_CHANGE_ID" ]; then
  MIGRATION_CHANGE_ID="$CLI_MIGRATION_CHANGE_ID"
fi

APP_ENV="${APP_ENV:-dev}"
RUN_MIGRATIONS="${RUN_MIGRATIONS:-auto}"
DB_URL="${DB_URL:-}"
MIGRATIONS_SKIP_ACK="${MIGRATIONS_SKIP_ACK:-false}"
MIGRATION_CHANGE_ID="${MIGRATION_CHANGE_ID:-}"

if [ "$APP_ENV" = "dev" ]; then
  echo "[migration-policy] APP_ENV=dev, skipping non-dev policy checks"
  exit 0
fi

require_skip_trace() {
  if [ "$MIGRATIONS_SKIP_ACK" != "true" ]; then
    echo "[migration-policy] skipping migrations requires MIGRATIONS_SKIP_ACK=true" >&2
    exit 1
  fi
  if [ -z "$MIGRATION_CHANGE_ID" ]; then
    echo "[migration-policy] skipping migrations requires MIGRATION_CHANGE_ID (ticket/change-id)" >&2
    exit 1
  fi
}

case "$RUN_MIGRATIONS" in
  true)
    if [ -z "$DB_URL" ]; then
      echo "[migration-policy] RUN_MIGRATIONS=true requires DB_URL" >&2
      exit 1
    fi
    ;;
  auto)
    if [ -z "$DB_URL" ]; then
      require_skip_trace
    fi
    ;;
  false)
    require_skip_trace
    ;;
  *)
    echo "[migration-policy] RUN_MIGRATIONS must be one of: auto|true|false" >&2
    exit 1
    ;;
esac

echo "[migration-policy] policy check passed"
