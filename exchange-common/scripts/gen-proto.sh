#!/bin/bash
# Proto 编译脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
PROTO_DIR="$ROOT_DIR/proto"
GEN_DIR="$ROOT_DIR/gen"

# 检查依赖
check_deps() {
    if ! command -v protoc &> /dev/null; then
        echo "Error: protoc not found. Install with: brew install protobuf"
        exit 1
    fi

    if ! command -v protoc-gen-go &> /dev/null; then
        echo "Installing protoc-gen-go..."
        go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    fi

    if ! command -v protoc-gen-go-grpc &> /dev/null; then
        echo "Installing protoc-gen-go-grpc..."
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
    fi
}

# 清理生成目录
clean() {
    echo "Cleaning generated files..."
    rm -rf "$GEN_DIR"
    mkdir -p "$GEN_DIR"
}

# 编译 Proto
compile() {
    echo "Compiling proto files..."

    # 下载 google/protobuf 依赖
    GOOGLE_PROTO_DIR="$ROOT_DIR/.proto-deps/google/protobuf"
    if [ ! -d "$GOOGLE_PROTO_DIR" ]; then
        echo "Downloading google/protobuf..."
        mkdir -p "$GOOGLE_PROTO_DIR"
        curl -sL "https://raw.githubusercontent.com/protocolbuffers/protobuf/main/src/google/protobuf/timestamp.proto" -o "$GOOGLE_PROTO_DIR/timestamp.proto"
    fi

    # 编译所有 proto 文件
    for proto_file in "$PROTO_DIR"/*.proto; do
        if [ -f "$proto_file" ]; then
            echo "  Compiling $(basename "$proto_file")..."
            protoc \
                --proto_path="$PROTO_DIR" \
                --proto_path="$ROOT_DIR/.proto-deps" \
                --go_out="$GEN_DIR" \
                --go_opt=paths=source_relative \
                "$proto_file"
        fi
    done

    echo "Done! Generated files in $GEN_DIR"
}

# 主函数
main() {
    case "${1:-all}" in
        clean)
            clean
            ;;
        deps)
            check_deps
            ;;
        all)
            check_deps
            clean
            compile
            ;;
        *)
            echo "Usage: $0 {all|clean|deps}"
            exit 1
            ;;
    esac
}

main "$@"
