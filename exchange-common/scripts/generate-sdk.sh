#!/bin/bash
# SDK generation script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$COMMON_DIR")"
SDK_DIR="$REPO_ROOT/sdk/typescript"
OUTPUT_DIR="$SDK_DIR/generated"
OPENAPI_GENERATOR_CMD=()
OPENAPI_GENERATOR_MODE=""
COLLECTED_SOURCES=()

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
    if command -v openapi-generator-cli &> /dev/null && command -v java &> /dev/null; then
        OPENAPI_GENERATOR_CMD=("openapi-generator-cli")
        OPENAPI_GENERATOR_MODE="local"
        return
    fi

    if command -v docker &> /dev/null; then
        if docker info >/dev/null 2>&1; then
            local uid gid
            uid="$(id -u)"
            gid="$(id -g)"
            OPENAPI_GENERATOR_CMD=("docker" "run" "--rm" "-u" "${uid}:${gid}" "-v" "$REPO_ROOT:/local" "-w" "/local" "openapitools/openapi-generator-cli")
            OPENAPI_GENERATOR_MODE="docker"
            return
        fi
    fi

    if command -v npx &> /dev/null && command -v java &> /dev/null; then
        OPENAPI_GENERATOR_CMD=("npx" "--yes" "@openapitools/openapi-generator-cli")
        OPENAPI_GENERATOR_MODE="npx"
        return
    fi

    local jar_path="$REPO_ROOT/.cache/openapi-generator-cli/openapi-generator-cli.jar"
    if [ -f "$jar_path" ]; then
        if ! command -v java &> /dev/null; then
            echo "Error: java not found for $jar_path" >&2
            exit 1
        fi
        OPENAPI_GENERATOR_CMD=("java" "-jar" "$jar_path")
        OPENAPI_GENERATOR_MODE="jar"
        return
    fi

    if command -v java &> /dev/null && command -v curl &> /dev/null; then
        local version="7.6.0"
        local jar_url="https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/${version}/openapi-generator-cli-${version}.jar"
        mkdir -p "$(dirname "$jar_path")"
        echo "Downloading openapi-generator-cli ${version}..."
        curl -fsSL "$jar_url" -o "$jar_path"
        OPENAPI_GENERATOR_CMD=("java" "-jar" "$jar_path")
        OPENAPI_GENERATOR_MODE="jar"
        return
    fi

    echo "Error: openapi-generator-cli not found. Install with: brew install openapi-generator, or ensure Docker is running or java+curl are available." >&2
    exit 1
}

collect_sources() {
    local input_dir="$1"
    COLLECTED_SOURCES=()

    if [ -n "$input_dir" ]; then
        if [ ! -d "$input_dir" ]; then
            echo "Error: input directory not found: $input_dir" >&2
            exit 1
        fi

        local found=0
        for openapi_file in "$input_dir"/*.yaml; do
            if [ -f "$openapi_file" ]; then
                COLLECTED_SOURCES+=("$openapi_file")
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
            COLLECTED_SOURCES+=("$openapi_file")
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
    local generator_input="$openapi_file"
    local generator_output="$out_dir"

    echo "Generating TypeScript SDK for $service_name"

    if [ "$OPENAPI_GENERATOR_MODE" = "docker" ]; then
        generator_input="${generator_input#$REPO_ROOT/}"
        generator_output="${generator_output#$REPO_ROOT/}"
    fi

    "${OPENAPI_GENERATOR_CMD[@]}" generate \
        -i "$generator_input" \
        -g typescript-fetch \
        -o "$generator_output" \
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

    collect_sources "$input_dir"

    local generated=0
    for openapi_file in "${COLLECTED_SOURCES[@]}"; do
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
