#!/bin/bash
# Contract packaging script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$COMMON_DIR")"
CONTRACTS_DIR="$REPO_ROOT/contracts"
VERSIONS_FILE="$CONTRACTS_DIR/versions.json"
PROTO_DIR="$COMMON_DIR/proto"

usage() {
    cat <<USAGE
Usage: $0 [--version VERSION] [--output DIR]

Options:
  --version VERSION  Package a specific version (default: versions.json current)
  --output DIR       Output directory (default: contracts/versions/VERSION)
  -h, --help         Show this help message
USAGE
}

check_deps() {
    if ! command -v python3 &> /dev/null; then
        echo "Error: python3 not found." >&2
        exit 1
    fi
}

get_current_version() {
    python3 - "$VERSIONS_FILE" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    data = json.load(f)
version = data.get('current', '')
print(version)
PY
}

copy_openapi() {
    local out_dir="$1"
    local openapi_out="$out_dir/openapi"
    mkdir -p "$openapi_out"

    local openapi_sources=(
        "$REPO_ROOT/exchange-gateway/api/openapi.yaml"
        "$REPO_ROOT/exchange-admin/api/openapi.yaml"
        "$REPO_ROOT/exchange-wallet/api/openapi.yaml"
    )

    local found=0
    for openapi_file in "${openapi_sources[@]}"; do
        if [ -f "$openapi_file" ]; then
            local service_dir
            service_dir="$(dirname "$(dirname "$openapi_file")")"
            local service
            service="$(basename "$service_dir")"
            cp "$openapi_file" "$openapi_out/${service}.yaml"
            echo "  Copied OpenAPI: ${service}.yaml"
            found=1
        else
            echo "Error: OpenAPI file not found: $openapi_file" >&2
            exit 1
        fi
    done

    if [ "$found" -eq 0 ]; then
        echo "Error: no OpenAPI sources found." >&2
        exit 1
    fi
}

copy_proto() {
    local out_dir="$1"
    local proto_out="$out_dir/proto"
    mkdir -p "$proto_out"

    local found=0
    for proto_file in "$PROTO_DIR"/*.proto; do
        if [ -f "$proto_file" ]; then
            cp "$proto_file" "$proto_out/"
            echo "  Copied Proto: $(basename "$proto_file")"
            found=1
        fi
    done

    if [ "$found" -eq 0 ]; then
        echo "Error: no proto sources found in $PROTO_DIR" >&2
        exit 1
    fi
}

main() {
    local version=""
    local output_dir=""

    while [ "$#" -gt 0 ]; do
        case "$1" in
            --version)
                version="$2"
                shift 2
                ;;
            --output)
                output_dir="$2"
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

    check_deps

    if [ ! -f "$VERSIONS_FILE" ]; then
        echo "Error: versions.json not found at $VERSIONS_FILE" >&2
        exit 1
    fi

    if [ -z "$version" ]; then
        version="$(get_current_version)"
    fi

    if [ -z "$version" ]; then
        echo "Error: version is empty; update $VERSIONS_FILE" >&2
        exit 1
    fi

    if [ -z "$output_dir" ]; then
        output_dir="$CONTRACTS_DIR/versions/$version"
    fi

    echo "Packaging contracts for $version"
    echo "Output: $output_dir"

    mkdir -p "$output_dir"
    copy_openapi "$output_dir"
    copy_proto "$output_dir"

    echo "Done! Packaged contracts in $output_dir"
}

main "$@"
