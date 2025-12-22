#!/bin/bash
# 启动所有服务脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
EXCHANGE_ROOT="$(dirname "$PROJECT_ROOT")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 服务列表
SERVICES=(
    "exchange-user:8085"
    "exchange-order:8081"
    "exchange-matching:8082"
    "exchange-clearing:8083"
    "exchange-marketdata:8084"
    "exchange-admin:8086"
    "exchange-wallet:8087"
    "exchange-gateway:8080"
)

start_infra() {
    log_info "Starting infrastructure (PostgreSQL, Redis)..."
    cd "$PROJECT_ROOT"
    docker-compose up -d postgres redis

    log_info "Waiting for PostgreSQL to be ready..."
    sleep 5

    # 初始化数据库
    if [ -f "scripts/init-db.sql" ]; then
        log_info "Initializing database schema..."
        docker-compose exec -T postgres psql -U exchange -d exchange < scripts/init-db.sql 2>/dev/null || true
    fi
}

build_service() {
    local service=$1
    local service_dir="${EXCHANGE_ROOT}/${service}"

    if [ -d "$service_dir" ]; then
        log_info "Building ${service}..."
        cd "$service_dir"
        go build -o "bin/${service##*-}" "./cmd/${service##*-}" 2>/dev/null || {
            log_warn "Failed to build ${service}, skipping..."
            return 1
        }
        return 0
    else
        log_warn "Service directory not found: ${service_dir}"
        return 1
    fi
}

start_service() {
    local service=$1
    local port=$2
    local service_dir="${EXCHANGE_ROOT}/${service}"
    local binary="bin/${service##*-}"

    if [ -f "${service_dir}/${binary}" ]; then
        log_info "Starting ${service} on port ${port}..."
        cd "$service_dir"
        "./${binary}" > "logs/${service##*-}.log" 2>&1 &
        echo $! > "pids/${service##*-}.pid"
        sleep 1

        # 健康检查
        if curl -sf "http://localhost:${port}/health" > /dev/null 2>&1; then
            log_info "${service} started successfully"
            return 0
        else
            log_warn "${service} may not be ready yet"
            return 0
        fi
    else
        log_error "Binary not found: ${service_dir}/${binary}"
        return 1
    fi
}

stop_all() {
    log_info "Stopping all services..."
    for entry in "${SERVICES[@]}"; do
        local service="${entry%%:*}"
        local service_dir="${EXCHANGE_ROOT}/${service}"
        local pid_file="${service_dir}/pids/${service##*-}.pid"

        if [ -f "$pid_file" ]; then
            local pid=$(cat "$pid_file")
            if kill -0 "$pid" 2>/dev/null; then
                log_info "Stopping ${service} (PID: ${pid})..."
                kill "$pid" 2>/dev/null || true
            fi
            rm -f "$pid_file"
        fi
    done

    log_info "Stopping infrastructure..."
    cd "$PROJECT_ROOT"
    docker-compose down 2>/dev/null || true
}

status() {
    log_info "Service Status:"
    for entry in "${SERVICES[@]}"; do
        local service="${entry%%:*}"
        local port="${entry##*:}"

        if curl -sf "http://localhost:${port}/health" > /dev/null 2>&1; then
            echo -e "  ${GREEN}●${NC} ${service} (port ${port})"
        else
            echo -e "  ${RED}○${NC} ${service} (port ${port})"
        fi
    done
}

main() {
    case "${1:-start}" in
        start)
            # 创建必要目录
            for entry in "${SERVICES[@]}"; do
                local service="${entry%%:*}"
                local service_dir="${EXCHANGE_ROOT}/${service}"
                mkdir -p "${service_dir}/logs" "${service_dir}/pids" "${service_dir}/bin"
            done

            start_infra

            log_info "Building and starting services..."
            for entry in "${SERVICES[@]}"; do
                local service="${entry%%:*}"
                local port="${entry##*:}"

                if build_service "$service"; then
                    start_service "$service" "$port"
                fi
            done

            echo ""
            status
            ;;
        stop)
            stop_all
            ;;
        restart)
            stop_all
            sleep 2
            main start
            ;;
        status)
            status
            ;;
        build)
            for entry in "${SERVICES[@]}"; do
                local service="${entry%%:*}"
                build_service "$service"
            done
            ;;
        *)
            echo "Usage: $0 {start|stop|restart|status|build}"
            exit 1
            ;;
    esac
}

main "$@"
