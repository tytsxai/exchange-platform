#!/bin/bash
set -euo pipefail

# Trim Redis Streams to prevent unbounded growth.
#
# Example:
#   STREAMS="exchange:orders,exchange:events,exchange:events:dlq" MAX_LEN=1000000 \
#     REDIS_ADDR=redis:6379 REDIS_PASSWORD=... bash exchange-common/scripts/trim-streams.sh

REDIS_ADDR=${REDIS_ADDR:-"localhost:6380"}
REDIS_USERNAME=${REDIS_USERNAME:-""}
REDIS_PASSWORD=${REDIS_PASSWORD:-""}
REDIS_TLS=${REDIS_TLS:-"false"}
REDIS_CACERT=${REDIS_CACERT:-""}
REDIS_CERT=${REDIS_CERT:-""}
REDIS_KEY=${REDIS_KEY:-""}

STREAMS=${STREAMS:-"exchange:orders,exchange:events"}
MAX_LEN=${MAX_LEN:-1000000}

if [ -z "$STREAMS" ]; then
  echo "STREAMS is empty; nothing to trim" >&2
  exit 1
fi

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli not found in PATH" >&2
  exit 1
fi

args=()
if [ -n "$REDIS_USERNAME" ]; then
  args+=("--user" "$REDIS_USERNAME")
fi
if [ "$REDIS_TLS" = "true" ] || [ "$REDIS_TLS" = "1" ]; then
  args+=("--tls")
fi
if [ -n "$REDIS_CACERT" ]; then
  args+=("--cacert" "$REDIS_CACERT")
fi
if [ -n "$REDIS_CERT" ]; then
  args+=("--cert" "$REDIS_CERT")
fi
if [ -n "$REDIS_KEY" ]; then
  args+=("--key" "$REDIS_KEY")
fi

if [ -n "$REDIS_PASSWORD" ]; then
  export REDISCLI_AUTH="$REDIS_PASSWORD"
fi

IFS=',' read -r -a stream_list <<< "$STREAMS"
for stream in "${stream_list[@]}"; do
  stream=$(echo "$stream" | xargs)
  if [ -z "$stream" ]; then
    continue
  fi
  echo "Trimming stream ${stream} to MAXLEN ~ ${MAX_LEN}..."
  redis-cli -h "${REDIS_ADDR%%:*}" -p "${REDIS_ADDR##*:}" "${args[@]}" XTRIM "$stream" MAXLEN "~" "$MAX_LEN" >/dev/null
done

echo "Trim complete."
