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
# - In non-dev, skipping migrations requires MIGRATIONS_SKIP_ACK=true.
# - In non-dev image-only mode, target images must be pullable (VERIFY_IMAGE_PULL=true).

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

# Preserve CLI/runtime overrides that may be shadowed by env file.
CLI_BUILD_IMAGES="${BUILD_IMAGES-}"
CLI_ALLOW_SOURCE_BUILD_IN_NONDEV="${ALLOW_SOURCE_BUILD_IN_NONDEV-}"
CLI_RUN_MIGRATIONS="${RUN_MIGRATIONS-}"
CLI_DRY_RUN="${DRY_RUN-}"
CLI_MIGRATIONS_SKIP_ACK="${MIGRATIONS_SKIP_ACK-}"
CLI_VERIFY_IMAGE_PULL="${VERIFY_IMAGE_PULL-}"

PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"
BUILD_IMAGES="${BUILD_IMAGES:-false}"
ALLOW_SOURCE_BUILD_IN_NONDEV="${ALLOW_SOURCE_BUILD_IN_NONDEV:-false}"
RUN_MIGRATIONS="${RUN_MIGRATIONS:-auto}"
DRY_RUN="${DRY_RUN:-false}"
MIGRATIONS_SKIP_ACK="${MIGRATIONS_SKIP_ACK:-false}"
VERIFY_IMAGE_PULL="${VERIFY_IMAGE_PULL:-true}"

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

if [ -n "$CLI_BUILD_IMAGES" ]; then
  BUILD_IMAGES="$CLI_BUILD_IMAGES"
fi
if [ -n "$CLI_ALLOW_SOURCE_BUILD_IN_NONDEV" ]; then
  ALLOW_SOURCE_BUILD_IN_NONDEV="$CLI_ALLOW_SOURCE_BUILD_IN_NONDEV"
fi
if [ -n "$CLI_RUN_MIGRATIONS" ]; then
  RUN_MIGRATIONS="$CLI_RUN_MIGRATIONS"
fi
if [ -n "$CLI_DRY_RUN" ]; then
  DRY_RUN="$CLI_DRY_RUN"
fi
if [ -n "$CLI_MIGRATIONS_SKIP_ACK" ]; then
  MIGRATIONS_SKIP_ACK="$CLI_MIGRATIONS_SKIP_ACK"
fi
if [ -n "$CLI_VERIFY_IMAGE_PULL" ]; then
  VERIFY_IMAGE_PULL="$CLI_VERIFY_IMAGE_PULL"
fi

echo "[deploy] running preflight (env: ${PROD_ENV_FILE})..."
(cd "$ROOT_DIR" && bash exchange-common/scripts/prod-preflight.sh)

APP_ENV="${APP_ENV:-dev}"
if [ "$APP_ENV" != "dev" ] && [ "$BUILD_IMAGES" = "true" ] && [ "$ALLOW_SOURCE_BUILD_IN_NONDEV" != "true" ]; then
  echo "[deploy] BUILD_IMAGES=true is blocked for non-dev by default." >&2
  echo "[deploy] Reason: source builds make APP_VERSION rollback non-immutable." >&2
  echo "[deploy] Set ALLOW_SOURCE_BUILD_IN_NONDEV=true only for emergency/manual scenarios." >&2
  exit 1
fi

if [ "$APP_ENV" != "dev" ] && [ "$BUILD_IMAGES" != "true" ] && [ "$VERIFY_IMAGE_PULL" = "true" ]; then
  echo "[deploy] verifying target image tags are pullable..."
  (cd "$ROOT_DIR" && PROD_ENV_FILE="$PROD_ENV_FILE" DRY_RUN="$DRY_RUN" VERIFY_IMAGE_PULL="$VERIFY_IMAGE_PULL" bash deploy/prod/check-images.sh)
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
  if [ "$APP_ENV" != "dev" ] && [ "$MIGRATIONS_SKIP_ACK" != "true" ]; then
    echo "[deploy] refusing to skip DB migrations in non-dev without explicit acknowledgement." >&2
    echo "[deploy] Set DB_URL (and keep RUN_MIGRATIONS=auto/true) to run migrations during deploy." >&2
    echo "[deploy] If migrations are already applied, set MIGRATIONS_SKIP_ACK=true to proceed." >&2
    exit 1
  fi
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
