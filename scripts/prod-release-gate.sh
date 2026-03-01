#!/bin/bash
set -euo pipefail

# Production release gate:
# 1) prod preflight
# 2) migration strategy policy
# 3) image availability check
# 4) public exposure policy
# 5) alertmanager routing config check
# 6) runtime verification against deployed endpoints (optional)

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

CLI_RUN_MIGRATION_POLICY_CHECK="${RUN_MIGRATION_POLICY_CHECK-}"
CLI_RUN_IMAGE_CHECK="${RUN_IMAGE_CHECK-}"
CLI_RUN_PUBLIC_EXPOSURE_CHECK="${RUN_PUBLIC_EXPOSURE_CHECK-}"
CLI_RUN_ALERTMANAGER_CHECK="${RUN_ALERTMANAGER_CHECK-}"
CLI_RUN_PROD_VERIFY="${RUN_PROD_VERIFY-}"

PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"
RUN_MIGRATION_POLICY_CHECK="${RUN_MIGRATION_POLICY_CHECK:-1}"
RUN_IMAGE_CHECK="${RUN_IMAGE_CHECK:-1}"
RUN_PUBLIC_EXPOSURE_CHECK="${RUN_PUBLIC_EXPOSURE_CHECK:-1}"
RUN_ALERTMANAGER_CHECK="${RUN_ALERTMANAGER_CHECK:-1}"
RUN_PROD_VERIFY="${RUN_PROD_VERIFY:-1}"

ENV_FILE_PATH="$PROD_ENV_FILE"
if [ ! -f "$ENV_FILE_PATH" ]; then
  ENV_FILE_PATH="$ROOT_DIR/$PROD_ENV_FILE"
fi
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "[release-gate] env file not found: $PROD_ENV_FILE" >&2
  exit 1
fi

set -a
. "$ENV_FILE_PATH"
set +a

if [ -n "$CLI_RUN_MIGRATION_POLICY_CHECK" ]; then
  RUN_MIGRATION_POLICY_CHECK="$CLI_RUN_MIGRATION_POLICY_CHECK"
fi
if [ -n "$CLI_RUN_IMAGE_CHECK" ]; then
  RUN_IMAGE_CHECK="$CLI_RUN_IMAGE_CHECK"
fi
if [ -n "$CLI_RUN_PUBLIC_EXPOSURE_CHECK" ]; then
  RUN_PUBLIC_EXPOSURE_CHECK="$CLI_RUN_PUBLIC_EXPOSURE_CHECK"
fi
if [ -n "$CLI_RUN_ALERTMANAGER_CHECK" ]; then
  RUN_ALERTMANAGER_CHECK="$CLI_RUN_ALERTMANAGER_CHECK"
fi
if [ -n "$CLI_RUN_PROD_VERIFY" ]; then
  RUN_PROD_VERIFY="$CLI_RUN_PROD_VERIFY"
fi

echo "[release-gate] 1/6 running prod-preflight"
(cd "$ROOT_DIR" && bash exchange-common/scripts/prod-preflight.sh)

if [ "$RUN_MIGRATION_POLICY_CHECK" = "1" ]; then
  echo "[release-gate] 2/6 running migration policy check"
  (cd "$ROOT_DIR" && PROD_ENV_FILE="$PROD_ENV_FILE" bash scripts/migration-policy-check.sh)
else
  echo "[release-gate] 2/6 migration policy check skipped"
fi

if [ "$RUN_IMAGE_CHECK" = "1" ]; then
  echo "[release-gate] 3/6 running image pullability check"
  (cd "$ROOT_DIR" && PROD_ENV_FILE="$PROD_ENV_FILE" bash deploy/prod/check-images.sh)
else
  echo "[release-gate] 3/6 image check skipped"
fi

if [ "$RUN_PUBLIC_EXPOSURE_CHECK" = "1" ]; then
  echo "[release-gate] 4/6 running public exposure policy check"
  (cd "$ROOT_DIR" && COMPOSE_FILE=deploy/prod/docker-compose.yml ALLOW_MARKETDATA_PUBLIC_WS="${ALLOW_MARKETDATA_PUBLIC_WS:-false}" bash scripts/check-public-exposure.sh)
else
  echo "[release-gate] 4/6 public exposure check skipped"
fi

if [ "$RUN_ALERTMANAGER_CHECK" = "1" ]; then
  echo "[release-gate] 5/6 running alertmanager config check"
  (cd "$ROOT_DIR" && ALERTMANAGER_CONFIG_FILE="${ALERTMANAGER_CONFIG_FILE:-deploy/prod/alertmanager.yml}" ALLOW_PLACEHOLDER_ALERTMANAGER="${ALLOW_PLACEHOLDER_ALERTMANAGER:-false}" REQUIRE_ONCALL_WEBHOOK="${REQUIRE_ONCALL_WEBHOOK:-true}" bash scripts/check-alertmanager-config.sh)
else
  echo "[release-gate] 5/6 alertmanager config check skipped"
fi

if [ "$RUN_PROD_VERIFY" = "1" ]; then
  echo "[release-gate] 6/6 running deployed runtime verification"
  (cd "$ROOT_DIR" && bash scripts/prod-verify.sh)
else
  echo "[release-gate] 6/6 runtime verification skipped"
fi

echo "[release-gate] all enabled checks passed"
