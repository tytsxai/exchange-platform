#!/bin/bash
set -euo pipefail

# Verify target images are pullable before deploy/rollback.
#
# Usage:
#   PROD_ENV_FILE=deploy/prod/prod.env bash deploy/prod/check-images.sh
#
# Optional env:
#   SERVICE_IMAGES="gateway user order matching clearing marketdata admin wallet"
#   VERIFY_IMAGE_PULL=true|false
#   IMAGE_REPOSITORY_PREFIX=ghcr.io/acme/   # must end with "/"
#   APP_VERSION=2026-02-25-rc1
#   DRY_RUN=true|false

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

CLI_APP_VERSION="${APP_VERSION-}"
CLI_IMAGE_REPOSITORY_PREFIX="${IMAGE_REPOSITORY_PREFIX-}"
CLI_SERVICE_IMAGES="${SERVICE_IMAGES-}"
CLI_VERIFY_IMAGE_PULL="${VERIFY_IMAGE_PULL-}"
CLI_DRY_RUN="${DRY_RUN-}"

PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"
SERVICE_IMAGES="${SERVICE_IMAGES:-gateway user order matching clearing marketdata admin wallet}"
VERIFY_IMAGE_PULL="${VERIFY_IMAGE_PULL:-true}"
DRY_RUN="${DRY_RUN:-false}"

ENV_FILE_PATH="$PROD_ENV_FILE"
if [ ! -f "$ENV_FILE_PATH" ]; then
  ENV_FILE_PATH="$ROOT_DIR/$PROD_ENV_FILE"
fi
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "[check-images] env file not found: $PROD_ENV_FILE" >&2
  exit 1
fi

set -a
. "$ENV_FILE_PATH"
set +a

if [ -n "$CLI_APP_VERSION" ]; then
  APP_VERSION="$CLI_APP_VERSION"
fi
if [ -n "$CLI_IMAGE_REPOSITORY_PREFIX" ]; then
  IMAGE_REPOSITORY_PREFIX="$CLI_IMAGE_REPOSITORY_PREFIX"
fi
if [ -n "$CLI_SERVICE_IMAGES" ]; then
  SERVICE_IMAGES="$CLI_SERVICE_IMAGES"
fi
if [ -n "$CLI_VERIFY_IMAGE_PULL" ]; then
  VERIFY_IMAGE_PULL="$CLI_VERIFY_IMAGE_PULL"
fi
if [ -n "$CLI_DRY_RUN" ]; then
  DRY_RUN="$CLI_DRY_RUN"
fi

if [ "${APP_VERSION:-}" = "" ]; then
  echo "[check-images] APP_VERSION is required" >&2
  exit 1
fi

case "$(echo "$VERIFY_IMAGE_PULL" | tr '[:upper:]' '[:lower:]')" in
  true|1)
    VERIFY_IMAGE_PULL="true"
    ;;
  false|0)
    VERIFY_IMAGE_PULL="false"
    ;;
  *)
    echo "[check-images] VERIFY_IMAGE_PULL must be true/false (or 1/0)" >&2
    exit 1
    ;;
esac

if [ "$VERIFY_IMAGE_PULL" != "true" ]; then
  echo "[check-images] VERIFY_IMAGE_PULL=false, skipping pull checks"
  exit 0
fi

prefix="${IMAGE_REPOSITORY_PREFIX:-}"
if [ -n "$prefix" ] && [ "${prefix%/}" = "$prefix" ]; then
  echo "[check-images] IMAGE_REPOSITORY_PREFIX must end with '/': ${prefix}" >&2
  exit 1
fi

for svc in $SERVICE_IMAGES; do
  image="${prefix}exchange-${svc}:${APP_VERSION}"
  if [ "$DRY_RUN" = "true" ]; then
    echo "[check-images] dry-run: docker pull $image"
    continue
  fi

  echo "[check-images] pulling $image"
  if ! docker pull "$image" >/dev/null; then
    echo "[check-images] failed to pull: $image" >&2
    exit 1
  fi
done

echo "[check-images] all images are pullable"
