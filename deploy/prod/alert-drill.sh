#!/bin/bash
set -euo pipefail

# Trigger or resolve a synthetic alert in Alertmanager.
#
# Usage:
#   bash deploy/prod/alert-drill.sh fire
#   DRILL_ID=manual-20260225T120000Z bash deploy/prod/alert-drill.sh resolve
#
# Optional env:
#   ALERTMANAGER_URL=http://localhost:9093
#   ALERT_NAME=ManualOncallDrill
#   TARGET_INSTANCE=exchange-prod
#   ALERT_SEVERITY=critical
#   DRILL_ID=<shared id for fire/resolve>

MODE="${1:-fire}"
ALERTMANAGER_URL="${ALERTMANAGER_URL:-http://localhost:9093}"
ALERT_NAME="${ALERT_NAME:-ManualOncallDrill}"
TARGET_INSTANCE="${TARGET_INSTANCE:-exchange-prod}"
ALERT_SEVERITY="${ALERT_SEVERITY:-critical}"
DRILL_ID="${DRILL_ID:-manual-$(date -u +%Y%m%dT%H%M%SZ)}"

case "$MODE" in
  fire|resolve)
    ;;
  *)
    echo "Usage: $0 [fire|resolve]" >&2
    exit 1
    ;;
esac

tmp_payload="$(mktemp)"
trap 'rm -f "$tmp_payload"' EXIT

now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ends_at=""
if [ "$MODE" = "resolve" ]; then
  ends_at=", \"endsAt\": \"$now\""
fi

cat >"$tmp_payload" <<EOF
[
  {
    "labels": {
      "alertname": "$ALERT_NAME",
      "severity": "$ALERT_SEVERITY",
      "instance": "$TARGET_INSTANCE",
      "drill_id": "$DRILL_ID",
      "source": "manual-drill"
    },
    "annotations": {
      "summary": "Manual alert drill ($MODE)",
      "description": "Synthetic alert for on-call notification drill"
    },
    "startsAt": "$now"$ends_at
  }
]
EOF

curl -fsS -H "Content-Type: application/json" -XPOST \
  "$ALERTMANAGER_URL/api/v2/alerts" \
  --data-binary @"$tmp_payload" >/dev/null

if [ "$MODE" = "fire" ]; then
  echo "[alert-drill] fired alert: $ALERT_NAME drill_id=$DRILL_ID"
  echo "[alert-drill] resolve command:"
  echo "DRILL_ID=$DRILL_ID ALERT_NAME=$ALERT_NAME TARGET_INSTANCE=$TARGET_INSTANCE ALERTMANAGER_URL=$ALERTMANAGER_URL bash deploy/prod/alert-drill.sh resolve"
else
  echo "[alert-drill] resolved alert: $ALERT_NAME drill_id=$DRILL_ID"
fi
