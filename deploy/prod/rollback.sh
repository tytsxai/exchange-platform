#!/bin/sh
set -eu

# Minimal rollback helper for Docker Compose production deployment.
#
# Usage:
#   APP_VERSION=<previous-tag> bash deploy/prod/rollback.sh
#
# Optional env:
#   PROD_ENV_FILE=deploy/prod/prod.env
#   ROLLBACK_VERSION=<previous-tag>   # fallback if APP_VERSION not set

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"
DRY_RUN="${DRY_RUN:-false}"

# Preserve CLI/runtime overrides that may be shadowed by env file.
CLI_APP_VERSION="${APP_VERSION-}"
CLI_ROLLBACK_VERSION="${ROLLBACK_VERSION-}"

ENV_FILE_PATH="$PROD_ENV_FILE"
if [ ! -f "$ENV_FILE_PATH" ]; then
  ENV_FILE_PATH="$ROOT_DIR/$PROD_ENV_FILE"
fi
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "[rollback] env file not found: $PROD_ENV_FILE" >&2
  exit 1
fi

set -a
. "$ENV_FILE_PATH"
set +a

if [ -n "$CLI_APP_VERSION" ]; then
  APP_VERSION="$CLI_APP_VERSION"
fi
if [ -n "$CLI_ROLLBACK_VERSION" ]; then
  ROLLBACK_VERSION="$CLI_ROLLBACK_VERSION"
fi

APP_ENV="${APP_ENV:-dev}"
TARGET_VERSION="${APP_VERSION:-${ROLLBACK_VERSION:-}}"

if [ "$TARGET_VERSION" = "" ]; then
  echo "[rollback] APP_VERSION (or ROLLBACK_VERSION) is required" >&2
  exit 1
fi

if [ "$APP_ENV" != "dev" ] && [ "$TARGET_VERSION" = "latest" ]; then
  echo "[rollback] refusing to rollback to APP_VERSION=latest in non-dev" >&2
  exit 1
fi

echo "[rollback] running preflight (env: ${PROD_ENV_FILE}, APP_VERSION=${TARGET_VERSION})..."
(cd "$ROOT_DIR" && APP_VERSION="$TARGET_VERSION" bash exchange-common/scripts/prod-preflight.sh)

echo "[rollback] deploying image tag: ${TARGET_VERSION}"
cd "$ROOT_DIR"
if [ "$DRY_RUN" = "true" ]; then
  echo "[rollback] dry-run: APP_VERSION=${TARGET_VERSION} docker compose -f deploy/prod/docker-compose.yml --env-file ${ENV_FILE_PATH} up -d"
  echo "[rollback] dry-run done"
  exit 0
fi

APP_VERSION="$TARGET_VERSION" docker compose -f deploy/prod/docker-compose.yml --env-file "$ENV_FILE_PATH" up -d

echo "[rollback] done"
