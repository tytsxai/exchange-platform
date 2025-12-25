#!/bin/sh
set -eu

# Minimal production deploy helper (docker compose).
#
# Usage:
#   cp deploy/prod/prod.env.example deploy/prod/prod.env
#   # (optional) export DB_URL=postgres://... to run migrations
#   bash deploy/prod/deploy.sh
#
# Notes:
# - This script intentionally avoids managing Postgres/Redis containers in production.
# - It runs fast-fail preflight checks to prevent dev defaults from reaching prod.

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"

echo "[deploy] running preflight (env: ${PROD_ENV_FILE})..."
(cd "$ROOT_DIR" && set -a && . "$PROD_ENV_FILE" && set +a && bash exchange-common/scripts/prod-preflight.sh)

if [ "${DB_URL:-}" != "" ]; then
  echo "[deploy] running DB migrations via exchange-common/scripts/migrate.sh..."
  (cd "$ROOT_DIR/exchange-common" && DB_URL="$DB_URL" bash scripts/migrate.sh)
else
  echo "[deploy] DB_URL not set; skipping migrations"
fi

echo "[deploy] starting services..."
cd "$ROOT_DIR"
docker compose -f deploy/prod/docker-compose.yml --env-file "$PROD_ENV_FILE" up -d --build

echo "[deploy] done"
