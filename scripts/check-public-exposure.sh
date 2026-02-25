#!/bin/bash
set -euo pipefail

# Validate docker-compose port exposure policy for production.
#
# Default policy:
# - Public ports are allowed only on gateway (8080/8090).
# - marketdata (8094) is allowed only when ALLOW_MARKETDATA_PUBLIC_WS=true.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-deploy/prod/docker-compose.yml}"
ALLOW_MARKETDATA_PUBLIC_WS="${ALLOW_MARKETDATA_PUBLIC_WS:-false}"
REQUIRE_GATEWAY_WS_PORT="${REQUIRE_GATEWAY_WS_PORT:-true}"

COMPOSE_FILE_PATH="$COMPOSE_FILE"
if [ ! -f "$COMPOSE_FILE_PATH" ]; then
  COMPOSE_FILE_PATH="$ROOT_DIR/$COMPOSE_FILE"
fi
if [ ! -f "$COMPOSE_FILE_PATH" ]; then
  echo "[exposure-check] compose file not found: $COMPOSE_FILE" >&2
  exit 1
fi

exposure_tmp="$(mktemp)"
trap 'rm -f "$exposure_tmp"' EXIT

in_services=0
in_ports=0
svc=""

while IFS= read -r line; do
  if [ "$line" = "services:" ]; then
    in_services=1
    continue
  fi

  if [ "$in_services" -eq 0 ]; then
    continue
  fi

  # End of top-level services block.
  case "$line" in
    [![:space:]]*)
      in_services=0
      in_ports=0
      continue
      ;;
  esac

  if [[ "$line" =~ ^[[:space:]]{2}([A-Za-z0-9_-]+):[[:space:]]*$ ]]; then
    svc="${BASH_REMATCH[1]}"
    in_ports=0
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]{4}ports:[[:space:]]*$ ]]; then
    in_ports=1
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]{4}[A-Za-z0-9_-]+: ]] && [[ ! "$line" =~ ^[[:space:]]{4}ports:[[:space:]]*$ ]]; then
    in_ports=0
    continue
  fi

  if [ "$in_ports" -eq 1 ] && [[ "$line" =~ ^[[:space:]]{6}-[[:space:]] ]]; then
    entry="$(echo "$line" | sed -E 's/^[[:space:]]*-[[:space:]]*"?([^"]+)"?.*/\1/')"
    IFS=':' read -r -a port_parts <<< "$entry"
    part_count="${#port_parts[@]}"
    if [ "$part_count" -ge 2 ]; then
      host_port="${port_parts[$((part_count - 2))]}"
      container_port="${port_parts[$((part_count - 1))]}"
      if [[ "$host_port" =~ ^[0-9]+$ ]] && [[ "$container_port" =~ ^[0-9]+$ ]]; then
        echo "$svc $host_port $container_port" >>"$exposure_tmp"
      fi
    fi
  fi
done <"$COMPOSE_FILE_PATH"

has_gateway_8080=0
has_gateway_8090=0

while IFS=' ' read -r svc host_port container_port; do
  [ -n "${svc:-}" ] || continue

  case "$svc" in
    gateway)
      if [ "$host_port" = "8080" ]; then
        has_gateway_8080=1
      elif [ "$host_port" = "8090" ]; then
        has_gateway_8090=1
      else
        echo "[exposure-check] disallowed gateway host port: $host_port" >&2
        exit 1
      fi
      ;;
    marketdata)
      if [ "$ALLOW_MARKETDATA_PUBLIC_WS" != "true" ]; then
        echo "[exposure-check] marketdata port exposed but ALLOW_MARKETDATA_PUBLIC_WS=false" >&2
        exit 1
      fi
      if [ "$host_port" != "8094" ] || [ "$container_port" != "8094" ]; then
        echo "[exposure-check] marketdata exposure must be 8094:8094, got ${host_port}:${container_port}" >&2
        exit 1
      fi
      ;;
    *)
      echo "[exposure-check] disallowed public exposure on service '$svc' (${host_port}:${container_port})" >&2
      exit 1
      ;;
  esac
done <"$exposure_tmp"

if [ "$has_gateway_8080" -ne 1 ]; then
  echo "[exposure-check] gateway 8080:8080 exposure is required" >&2
  exit 1
fi

if [ "$REQUIRE_GATEWAY_WS_PORT" = "true" ] && [ "$has_gateway_8090" -ne 1 ]; then
  echo "[exposure-check] gateway 8090:8090 exposure is required (set REQUIRE_GATEWAY_WS_PORT=false to override)" >&2
  exit 1
fi

echo "[exposure-check] exposure policy check passed"
