#!/bin/bash
# SDK generation script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$COMMON_DIR")"
SDK_DIR="$REPO_ROOT/sdk/typescript"
OUTPUT_DIR="$SDK_DIR/generated"

usage() {
    cat <<USAGE
Usage: $0 [--input DIR] [--output DIR] [--service NAME] [--clean]

Options:
  --input DIR    OpenAPI source directory (default: exchange-*/api)
  --output DIR   Output directory (default: sdk/typescript/generated)
  --service NAME Generate a single service SDK (e.g. exchange-gateway)
  --clean        Remove the output directory before generation
  -h, --help     Show this help message
USAGE
}

check_deps() {
    if ! command -v openapi-generator-cli &> /dev/null; then
        echo "Error: openapi-generator-cli not found. Install with: brew install openapi-generator" >&2
        exit 1
    fi
}

collect_sources() {
    local input_dir="$1"
    local -n files_ref=$2

    if [ -n "$input_dir" ]; then
        if [ ! -d "$input_dir" ]; then
            echo "Error: input directory not found: $input_dir" >&2
            exit 1
        fi

        local found=0
        for openapi_file in "$input_dir"/*.yaml; do
            if [ -f "$openapi_file" ]; then
                files_ref+=("$openapi_file")
                found=1
            fi
        done

        if [ "$found" -eq 0 ]; then
            echo "Error: no OpenAPI files found in $input_dir" >&2
            exit 1
        fi
        return
    fi

    local default_sources=(
        "$REPO_ROOT/exchange-gateway/api/openapi.yaml"
        "$REPO_ROOT/exchange-admin/api/openapi.yaml"
        "$REPO_ROOT/exchange-wallet/api/openapi.yaml"
    )

    for openapi_file in "${default_sources[@]}"; do
        if [ -f "$openapi_file" ]; then
            files_ref+=("$openapi_file")
        else
            echo "Error: OpenAPI file not found: $openapi_file" >&2
            exit 1
        fi
    done
}

generate_sdk() {
    local openapi_file="$1"
    local service_name="$2"
    local out_dir="$OUTPUT_DIR/$service_name"

    echo "Generating TypeScript SDK for $service_name"

    openapi-generator-cli generate \
        -i "$openapi_file" \
        -g typescript-fetch \
        -o "$out_dir" \
        --additional-properties=typescriptThreePlus=true
}

main() {
    local input_dir=""
    local output_dir_override=""
    local service_filter=""
    local clean=false

    while [ "$#" -gt 0 ]; do
        case "$1" in
            --input)
                input_dir="$2"
                shift 2
                ;;
            --output)
                output_dir_override="$2"
                shift 2
                ;;
            --service)
                service_filter="$2"
                shift 2
                ;;
            --clean)
                clean=true
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

    check_deps

    if [ -n "$output_dir_override" ]; then
        OUTPUT_DIR="$output_dir_override"
    fi

    if [ "$clean" = true ]; then
        echo "Cleaning output directory..."
        rm -rf "$OUTPUT_DIR"
    fi

    mkdir -p "$OUTPUT_DIR"

    local sources=()
    collect_sources "$input_dir" sources

    local generated=0
    for openapi_file in "${sources[@]}"; do
        local service_name=""
        if [ -n "$input_dir" ]; then
            service_name="$(basename "$openapi_file" .yaml)"
        else
            service_name="$(basename "$(dirname "$(dirname "$openapi_file")")")"
        fi

        if [ -n "$service_filter" ] && [ "$service_filter" != "$service_name" ]; then
            continue
        fi

        generate_sdk "$openapi_file" "$service_name"
        generated=$((generated + 1))
    done

    if [ "$generated" -eq 0 ]; then
        echo "Error: no SDKs generated. Check --service filter." >&2
        exit 1
    fi

    echo "Done! Generated SDKs in $OUTPUT_DIR"
}

main "$@"
