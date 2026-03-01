#!/bin/bash
set -euo pipefail

# Validate Alertmanager routing config is production-ready.
#
# Default policy:
# - alertmanager.yml must not contain the repository placeholder webhook URL.
# - At least one webhook receiver URL should exist.
#
# Optional env:
#   ALERTMANAGER_CONFIG_FILE=deploy/prod/alertmanager.yml
#   ALLOW_PLACEHOLDER_ALERTMANAGER=false
#   REQUIRE_ONCALL_WEBHOOK=true

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ALERTMANAGER_CONFIG_FILE="${ALERTMANAGER_CONFIG_FILE:-deploy/prod/alertmanager.yml}"
ALLOW_PLACEHOLDER_ALERTMANAGER="${ALLOW_PLACEHOLDER_ALERTMANAGER:-false}"
REQUIRE_ONCALL_WEBHOOK="${REQUIRE_ONCALL_WEBHOOK:-true}"

CONFIG_PATH="$ALERTMANAGER_CONFIG_FILE"
if [ ! -f "$CONFIG_PATH" ]; then
  CONFIG_PATH="$ROOT_DIR/$ALERTMANAGER_CONFIG_FILE"
fi
if [ ! -f "$CONFIG_PATH" ]; then
  echo "[alertmanager-check] config file not found: $ALERTMANAGER_CONFIG_FILE" >&2
  exit 1
fi

case "$(echo "$ALLOW_PLACEHOLDER_ALERTMANAGER" | tr '[:upper:]' '[:lower:]')" in
  true|1)
    ALLOW_PLACEHOLDER_ALERTMANAGER="true"
    ;;
  false|0)
    ALLOW_PLACEHOLDER_ALERTMANAGER="false"
    ;;
  *)
    echo "[alertmanager-check] ALLOW_PLACEHOLDER_ALERTMANAGER must be true/false (or 1/0)" >&2
    exit 1
    ;;
esac

case "$(echo "$REQUIRE_ONCALL_WEBHOOK" | tr '[:upper:]' '[:lower:]')" in
  true|1)
    REQUIRE_ONCALL_WEBHOOK="true"
    ;;
  false|0)
    REQUIRE_ONCALL_WEBHOOK="false"
    ;;
  *)
    echo "[alertmanager-check] REQUIRE_ONCALL_WEBHOOK must be true/false (or 1/0)" >&2
    exit 1
    ;;
esac

placeholder_pattern='replace-with-your-alert-relay\.internal'
if [ "$ALLOW_PLACEHOLDER_ALERTMANAGER" != "true" ] && grep -Eq "$placeholder_pattern" "$CONFIG_PATH"; then
  echo "[alertmanager-check] placeholder webhook endpoint detected in $CONFIG_PATH" >&2
  echo "[alertmanager-check] replace deploy/prod/alertmanager.yml receiver URL with real on-call relay" >&2
  echo "[alertmanager-check] (or set ALLOW_PLACEHOLDER_ALERTMANAGER=true only for explicit non-prod drills)" >&2
  exit 1
fi

webhook_urls=()
while IFS= read -r line; do
  webhook_urls+=("$line")
done < <(sed -nE 's/^[[:space:]]*-[[:space:]]*url:[[:space:]]*"?([^"#]+)"?.*/\1/p' "$CONFIG_PATH")
if [ "$REQUIRE_ONCALL_WEBHOOK" = "true" ] && [ "${#webhook_urls[@]}" -eq 0 ]; then
  echo "[alertmanager-check] no webhook receiver URL found in $CONFIG_PATH" >&2
  exit 1
fi

for raw in "${webhook_urls[@]}"; do
  url="$(echo "$raw" | awk '{$1=$1; print}')"
  if [ -z "$url" ]; then
    continue
  fi
  if ! echo "$url" | grep -Eq '^https?://'; then
    echo "[alertmanager-check] unsupported webhook URL scheme: $url" >&2
    exit 1
  fi
done

echo "[alertmanager-check] config check passed"
