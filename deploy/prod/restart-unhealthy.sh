#!/bin/bash
set -euo pipefail

# Restart unhealthy services in docker compose production deployment.
#
# This is mainly for docker compose environments where unhealthy containers
# are not auto-restarted by default.
#
# Usage:
#   bash deploy/prod/restart-unhealthy.sh
#   DRY_RUN=true bash deploy/prod/restart-unhealthy.sh
#
# Optional env:
#   COMPOSE_FILE=deploy/prod/docker-compose.yml
#   PROD_ENV_FILE=deploy/prod/prod.env

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-deploy/prod/docker-compose.yml}"
PROD_ENV_FILE="${PROD_ENV_FILE:-deploy/prod/prod.env}"
DRY_RUN="${DRY_RUN:-false}"

COMPOSE_FILE_PATH="$COMPOSE_FILE"
if [ ! -f "$COMPOSE_FILE_PATH" ]; then
  COMPOSE_FILE_PATH="$ROOT_DIR/$COMPOSE_FILE"
fi
if [ ! -f "$COMPOSE_FILE_PATH" ]; then
  echo "[restart-unhealthy] compose file not found: $COMPOSE_FILE" >&2
  exit 1
fi

ENV_FILE_PATH="$PROD_ENV_FILE"
if [ ! -f "$ENV_FILE_PATH" ]; then
  ENV_FILE_PATH="$ROOT_DIR/$PROD_ENV_FILE"
fi
if [ ! -f "$ENV_FILE_PATH" ]; then
  echo "[restart-unhealthy] env file not found: $PROD_ENV_FILE" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "[restart-unhealthy] docker not found in PATH" >&2
  exit 1
fi

mapfile -t unhealthy_services < <(
  docker compose -f "$COMPOSE_FILE_PATH" --env-file "$ENV_FILE_PATH" ps --format '{{.Service}} {{.State}} {{.Health}}' \
    | awk '$2 == "running" && $3 == "unhealthy" {print $1}'
)

if [ "${#unhealthy_services[@]}" -eq 0 ]; then
  echo "[restart-unhealthy] no unhealthy services"
  exit 0
fi

echo "[restart-unhealthy] unhealthy services: ${unhealthy_services[*]}"
if [ "$DRY_RUN" = "true" ]; then
  echo "[restart-unhealthy] dry-run: docker compose -f $COMPOSE_FILE_PATH --env-file $ENV_FILE_PATH restart ${unhealthy_services[*]}"
  exit 0
fi

docker compose -f "$COMPOSE_FILE_PATH" --env-file "$ENV_FILE_PATH" restart "${unhealthy_services[@]}"
echo "[restart-unhealthy] restarted: ${unhealthy_services[*]}"
