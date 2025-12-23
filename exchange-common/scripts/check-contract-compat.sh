#!/bin/bash
# Contract compatibility check script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$COMMON_DIR")"
CONTRACTS_DIR="$REPO_ROOT/contracts"
VERSIONS_FILE="$CONTRACTS_DIR/versions.json"
PUBLISH_SCRIPT="$SCRIPT_DIR/publish-contracts.sh"

usage() {
    cat <<USAGE
Usage: $0 [--baseline VERSION] [--check]

Options:
  --baseline VERSION  Compare against a specific published version
  --check             Run compatibility check (default)
  -h, --help          Show this help message
USAGE
}

check_deps() {
    if ! command -v python3 &> /dev/null; then
        echo "Error: python3 not found." >&2
        exit 1
    fi

    if [ ! -f "$PUBLISH_SCRIPT" ]; then
        echo "Error: publish script not found at $PUBLISH_SCRIPT" >&2
        exit 1
    fi
}

get_versions() {
    python3 - "$VERSIONS_FILE" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    data = json.load(f)

current = data.get('current', '')
history = data.get('history', [])
versions = [item.get('version', '') for item in history if item.get('version')]

baseline = ''
if current in versions:
    idx = versions.index(current)
    if idx > 0:
        baseline = versions[idx - 1]
    else:
        baseline = current
elif versions:
    baseline = versions[-1]

print(current)
print(baseline)
PY
}

compare_dir() {
    local label="$1"
    local baseline_dir="$2"
    local current_dir="$3"

    if [ ! -d "$baseline_dir" ]; then
        echo "Error: baseline $label directory not found at $baseline_dir" >&2
        exit 1
    fi

    if [ ! -d "$current_dir" ]; then
        echo "Error: current $label directory not found at $current_dir" >&2
        exit 1
    fi

    echo "Comparing $label..."
    if ! diff -ru "$baseline_dir" "$current_dir"; then
        return 1
    fi

    return 0
}

main() {
    local baseline_version=""
    local run_check=true

    while [ "$#" -gt 0 ]; do
        case "$1" in
            --baseline)
                baseline_version="$2"
                shift 2
                ;;
            --check)
                run_check=true
                shift
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

    if [ "$run_check" != true ]; then
        usage
        exit 1
    fi

    check_deps

    if [ ! -f "$VERSIONS_FILE" ]; then
        echo "Error: versions.json not found at $VERSIONS_FILE" >&2
        exit 1
    fi

    current_version=""
    computed_baseline=""
    while IFS= read -r line; do
        if [ -z "$current_version" ]; then
            current_version="$line"
        else
            computed_baseline="$line"
            break
        fi
    done < <(get_versions)

    if [ -z "$current_version" ]; then
        echo "Error: current version is empty; update $VERSIONS_FILE" >&2
        exit 1
    fi

    if [ -z "$baseline_version" ]; then
        baseline_version="$computed_baseline"
    fi

    if [ -z "$baseline_version" ]; then
        echo "Error: baseline version is empty; update $VERSIONS_FILE" >&2
        exit 1
    fi

    local baseline_dir="$CONTRACTS_DIR/versions/$baseline_version"
    if [ ! -d "$baseline_dir" ]; then
        echo "Baseline contract bundle not found at $baseline_dir; skipping compatibility check."
        exit 0
    fi

    local temp_dir
    temp_dir="$(mktemp -d)"
    trap 'rm -rf "$temp_dir"' EXIT

    echo "Generating current contract bundle..."
    bash "$PUBLISH_SCRIPT" --version "$current_version" --output "$temp_dir" >/dev/null

    local failed=0

    if ! compare_dir "openapi" "$baseline_dir/openapi" "$temp_dir/openapi"; then
        failed=1
    fi

    if ! compare_dir "proto" "$baseline_dir/proto" "$temp_dir/proto"; then
        failed=1
    fi

    if [ "$failed" -ne 0 ]; then
        echo "Contract compatibility check failed against $baseline_version." >&2
        exit 1
    fi

    echo "OK"
}

main "$@"
