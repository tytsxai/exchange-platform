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
BUILD_IMAGES="${BUILD_IMAGES:-false}"
ALLOW_SOURCE_BUILD_IN_NONDEV="${ALLOW_SOURCE_BUILD_IN_NONDEV:-false}"
RUN_MIGRATIONS="${RUN_MIGRATIONS:-auto}"
DRY_RUN="${DRY_RUN:-false}"

ENV_FILE_PATH="$PROD_ENV_FILE"
if [ ! -f "$ENV_FILE_PATH" ]; then
  ENV_FILE_PATH="$ROOT_DIR/$PROD_ENV_FILE"
fi
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "[deploy] env file not found: $PROD_ENV_FILE" >&2
  exit 1
fi

set -a
. "$ENV_FILE_PATH"
set +a

echo "[deploy] running preflight (env: ${PROD_ENV_FILE})..."
(cd "$ROOT_DIR" && bash exchange-common/scripts/prod-preflight.sh)

APP_ENV="${APP_ENV:-dev}"
if [ "$APP_ENV" != "dev" ] && [ "$BUILD_IMAGES" = "true" ] && [ "$ALLOW_SOURCE_BUILD_IN_NONDEV" != "true" ]; then
  echo "[deploy] BUILD_IMAGES=true is blocked for non-dev by default." >&2
  echo "[deploy] Reason: source builds make APP_VERSION rollback non-immutable." >&2
  echo "[deploy] Set ALLOW_SOURCE_BUILD_IN_NONDEV=true only for emergency/manual scenarios." >&2
  exit 1
fi

should_migrate="false"
if [ "$RUN_MIGRATIONS" = "true" ]; then
  should_migrate="true"
elif [ "$RUN_MIGRATIONS" = "auto" ] && [ "${DB_URL:-}" != "" ]; then
  should_migrate="true"
elif [ "$RUN_MIGRATIONS" != "auto" ] && [ "$RUN_MIGRATIONS" != "false" ]; then
  echo "[deploy] RUN_MIGRATIONS must be one of: auto|true|false" >&2
  exit 1
fi

if [ "$should_migrate" = "true" ]; then
  if [ "${DB_URL:-}" = "" ]; then
    echo "[deploy] RUN_MIGRATIONS=${RUN_MIGRATIONS} requires DB_URL" >&2
    exit 1
  fi
  echo "[deploy] running DB migrations via exchange-common/scripts/migrate.sh..."
  (cd "$ROOT_DIR/exchange-common" && APP_ENV="$APP_ENV" DB_URL="$DB_URL" bash scripts/migrate.sh)
else
  echo "[deploy] skipping migrations (RUN_MIGRATIONS=${RUN_MIGRATIONS})"
fi

echo "[deploy] starting services..."
cd "$ROOT_DIR"
if [ "$DRY_RUN" = "true" ]; then
  if [ "$BUILD_IMAGES" = "true" ]; then
    echo "[deploy] dry-run: docker compose -f deploy/prod/docker-compose.yml --env-file $ENV_FILE_PATH up -d --build"
  else
    echo "[deploy] dry-run: docker compose -f deploy/prod/docker-compose.yml --env-file $ENV_FILE_PATH up -d"
  fi
  echo "[deploy] dry-run done"
  exit 0
fi

if [ "$BUILD_IMAGES" = "true" ]; then
  echo "[deploy] mode=source-build (--build)"
  docker compose -f deploy/prod/docker-compose.yml --env-file "$ENV_FILE_PATH" up -d --build
else
  echo "[deploy] mode=image-only (no --build)"
  docker compose -f deploy/prod/docker-compose.yml --env-file "$ENV_FILE_PATH" up -d
fi

echo "[deploy] done"
