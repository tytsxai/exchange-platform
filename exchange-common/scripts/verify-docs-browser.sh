#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$COMMON_DIR")"
DOCS_DIR="$REPO_ROOT/docs"
PORT="${PORT:-4173}"
SERVER_LOG="${SERVER_LOG:-/tmp/contract-docs-server.log}"
PW_TEMP_DIR=""

usage() {
    cat <<USAGE
Usage: $0 [--port PORT]

Starts a local server for docs/contracts and runs a headless browser check.
USAGE
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --port)
            PORT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            exit 1
            ;;
    esac
done

if [ ! -d "$DOCS_DIR/contracts" ]; then
    echo "Error: docs/contracts not found. Run build-contract-docs.sh first." >&2
    exit 1
fi

python3 -m http.server "$PORT" --directory "$DOCS_DIR" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

cleanup() {
    kill "$SERVER_PID" 2>/dev/null || true
    if [ -n "$PW_TEMP_DIR" ]; then
        rm -rf "$PW_TEMP_DIR"
    fi
}
trap cleanup EXIT

sleep 1

PW_TEMP_DIR="$(mktemp -d)"
npm install --silent --prefix "$PW_TEMP_DIR" playwright@1.48.2
"$PW_TEMP_DIR/node_modules/.bin/playwright" install chromium >/dev/null
NODE_PATH="$PW_TEMP_DIR/node_modules" node "$SCRIPT_DIR/verify-docs-browser.js" \
    "http://localhost:$PORT/contracts/index.html"
