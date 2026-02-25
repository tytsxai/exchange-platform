#!/bin/bash
set -euo pipefail

# Production preflight checks (fast-fail).
#
# Usage:
#   APP_ENV=prod source /path/to/env && bash exchange-common/scripts/prod-preflight.sh
#
# Notes:
# - This script intentionally fails on common "dev defaults" to prevent accidental prod deploys.
# - It only validates env/config shape; it does not start services.

require_set() {
  local key="$1"
  if [ -z "${!key:-}" ]; then
    echo "Missing required env: ${key}" >&2
    exit 1
  fi
}

require_not_default() {
  local key="$1"
  local value="${!key:-}"
  local default="$2"
  if [ "$value" = "$default" ]; then
    echo "${key} must not be the dev placeholder value" >&2
    exit 1
  fi
}

require_not_equals() {
  local key="$1"
  local value="${!key:-}"
  local bad="$2"
  if [ "$value" = "$bad" ]; then
    echo "${key} must not be '${bad}'" >&2
    exit 1
  fi
}

APP_ENV="${APP_ENV:-dev}"
if [ "$APP_ENV" = "dev" ]; then
  echo "APP_ENV=dev detected; this preflight is intended for non-dev deployments." >&2
  exit 1
fi

MIN_SECRET_LENGTH="${MIN_SECRET_LENGTH:-32}"
APP_VERSION="${APP_VERSION:-latest}"
ALLOW_LATEST_IMAGE_TAG="${ALLOW_LATEST_IMAGE_TAG:-false}"
IMAGE_REPOSITORY_PREFIX="${IMAGE_REPOSITORY_PREFIX:-}"
VERIFY_IMAGE_PULL="${VERIFY_IMAGE_PULL:-true}"
ALLOW_MARKETDATA_PUBLIC_WS="${ALLOW_MARKETDATA_PUBLIC_WS:-false}"

if [ "$ALLOW_LATEST_IMAGE_TAG" != "true" ] && [ "$APP_VERSION" = "latest" ]; then
  echo "APP_VERSION must not be 'latest' in non-dev deployments (set ALLOW_LATEST_IMAGE_TAG=true to override)" >&2
  exit 1
fi

if [ -n "$IMAGE_REPOSITORY_PREFIX" ] && [ "${IMAGE_REPOSITORY_PREFIX%/}" = "$IMAGE_REPOSITORY_PREFIX" ]; then
  echo "IMAGE_REPOSITORY_PREFIX must end with '/'" >&2
  exit 1
fi

case "$(echo "$VERIFY_IMAGE_PULL" | tr '[:upper:]' '[:lower:]')" in
  true|1|false|0)
    ;;
  *)
    echo "VERIFY_IMAGE_PULL must be true/false (or 1/0)" >&2
    exit 1
    ;;
esac

case "$(echo "$ALLOW_MARKETDATA_PUBLIC_WS" | tr '[:upper:]' '[:lower:]')" in
  true|1|false|0)
    ;;
  *)
    echo "ALLOW_MARKETDATA_PUBLIC_WS must be true/false (or 1/0)" >&2
    exit 1
    ;;
esac

require_set INTERNAL_TOKEN
require_set AUTH_TOKEN_SECRET
require_set API_KEY_SECRET_KEY
require_set ADMIN_TOKEN

require_not_default INTERNAL_TOKEN "dev-internal-token-change-me"
require_not_default AUTH_TOKEN_SECRET "dev-auth-token-secret-32-bytes-minimum"
require_not_default API_KEY_SECRET_KEY "dev-api-key-secret-32-bytes-minimum"
require_not_default ADMIN_TOKEN "dev-admin-token-change-me"

if [ "${#INTERNAL_TOKEN}" -lt "$MIN_SECRET_LENGTH" ]; then
  echo "INTERNAL_TOKEN must be at least ${MIN_SECRET_LENGTH} characters" >&2
  exit 1
fi
if [ "${#AUTH_TOKEN_SECRET}" -lt "$MIN_SECRET_LENGTH" ]; then
  echo "AUTH_TOKEN_SECRET must be at least ${MIN_SECRET_LENGTH} characters" >&2
  exit 1
fi
if [ "${#API_KEY_SECRET_KEY}" -lt "$MIN_SECRET_LENGTH" ]; then
  echo "API_KEY_SECRET_KEY must be at least ${MIN_SECRET_LENGTH} characters" >&2
  exit 1
fi
if [ "${#ADMIN_TOKEN}" -lt "$MIN_SECRET_LENGTH" ]; then
  echo "ADMIN_TOKEN must be at least ${MIN_SECRET_LENGTH} characters" >&2
  exit 1
fi

require_set DB_HOST
require_set DB_PORT
require_set DB_USER
require_set DB_PASSWORD
require_set DB_NAME
require_set DB_SSL_MODE

require_not_equals DB_PASSWORD "exchange123"
if [ "$(echo "$DB_SSL_MODE" | tr '[:upper:]' '[:lower:]')" = "disable" ]; then
  echo "DB_SSL_MODE must not be 'disable'" >&2
  exit 1
fi

require_set REDIS_ADDR
require_set REDIS_PASSWORD

REDIS_TLS="${REDIS_TLS:-false}"
case "$(echo "$REDIS_TLS" | tr '[:upper:]' '[:lower:]')" in
  true|1|false|0)
    ;;
  *)
    echo "REDIS_TLS must be true/false (or 1/0)" >&2
    exit 1
    ;;
esac

redis_cert="${REDIS_CERT:-}"
redis_key="${REDIS_KEY:-}"
if { [ -n "$redis_cert" ] && [ -z "$redis_key" ]; } || { [ -z "$redis_cert" ] && [ -n "$redis_key" ]; }; then
  echo "REDIS_CERT and REDIS_KEY must be set together" >&2
  exit 1
fi

ENABLE_DOCS="${ENABLE_DOCS:-false}"
ALLOW_DOCS_IN_NONDEV="${ALLOW_DOCS_IN_NONDEV:-false}"
if [ "$ENABLE_DOCS" = "true" ] && [ "$ALLOW_DOCS_IN_NONDEV" != "true" ]; then
  echo "ENABLE_DOCS=true requires ALLOW_DOCS_IN_NONDEV=true when APP_ENV=${APP_ENV}" >&2
  exit 1
fi

CORS_ALLOW_ORIGINS="${CORS_ALLOW_ORIGINS:-}"
if [ -n "$CORS_ALLOW_ORIGINS" ] && echo "$CORS_ALLOW_ORIGINS" | tr ',' '\n' | awk '{gsub(/^[ \t]+|[ \t]+$/, "", $0); print}' | grep -qx '\*'; then
  echo "CORS_ALLOW_ORIGINS must not include '*' when APP_ENV=${APP_ENV}" >&2
  exit 1
fi

echo "Preflight OK."
