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

# Dev defaults (override in shell for production)
APP_ENV=${APP_ENV:-"dev"}
INTERNAL_TOKEN=${INTERNAL_TOKEN:-"dev-internal-token-change-me"}
AUTH_TOKEN_SECRET=${AUTH_TOKEN_SECRET:-"dev-auth-token-secret-32-bytes-minimum"}
AUTH_TOKEN_TTL=${AUTH_TOKEN_TTL:-"24h"}
ADMIN_TOKEN=${ADMIN_TOKEN:-"dev-admin-token-change-me"}

export APP_ENV INTERNAL_TOKEN AUTH_TOKEN_SECRET AUTH_TOKEN_TTL ADMIN_TOKEN

# 服务列表
SERVICES=(
    "exchange-user:8085"
    "exchange-order:8081"
    "exchange-matching:8082"
    "exchange-clearing:8083"
    "exchange-marketdata:8084"
    "exchange-admin:8087"
    "exchange-wallet:8086"
    "exchange-gateway:8080"
)

start_infra() {
    log_info "Starting infrastructure (PostgreSQL, Redis)..."
    cd "$PROJECT_ROOT"
    docker-compose up -d postgres redis

    log_info "Waiting for PostgreSQL to be ready..."
    sleep 5

    # 初始化数据库/迁移
    if [ -f "scripts/migrate.sh" ]; then
        log_info "Applying database migrations..."
        DB_URL=${DB_URL:-"postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"} \
            bash scripts/migrate.sh 2>/dev/null || true
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

pid_listening_on_port() {
    local port=$1
    lsof -t -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null | head -n 1
}

proc_name() {
    local pid=$1
    local comm
    comm=$(ps -o comm= -p "${pid}" 2>/dev/null | tr -d ' ')
    basename "${comm}"
}

start_service() {
    local service=$1
    local port=$2
    local service_dir="${EXCHANGE_ROOT}/${service}"
    local binary="bin/${service##*-}"
    local expected_comm="${service##*-}"

    if [ -f "${service_dir}/${binary}" ]; then
        local existing_pid
        existing_pid=$(pid_listening_on_port "${port}" || true)
        if [ -n "${existing_pid}" ]; then
            log_error "Port ${port} is already in use by PID ${existing_pid} (comm=$(proc_name "${existing_pid}")). Refusing to start ${service}."
            return 1
        fi

        log_info "Starting ${service} on port ${port}..."
        cd "$service_dir"
        "./${binary}" > "logs/${service##*-}.log" 2>&1 &
        echo $! > "pids/${service##*-}.pid"
        sleep 1

        if ! kill -0 "$!" 2>/dev/null; then
            log_error "${service} exited immediately. See ${service_dir}/logs/${service##*-}.log"
            return 1
        fi

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

stop_service() {
    local service=$1
    local port=$2
    local service_dir="${EXCHANGE_ROOT}/${service}"
    local pid_file="${service_dir}/pids/${service##*-}.pid"
    local expected_comm="${service##*-}"

    if [ -f "$pid_file" ]; then
        local pid
        pid=$(cat "$pid_file" 2>/dev/null || true)
        if [ -n "${pid}" ] && kill -0 "$pid" 2>/dev/null; then
            log_info "Stopping ${service} (PID: ${pid})..."
            kill "$pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi

    local listen_pid
    listen_pid=$(pid_listening_on_port "${port}" || true)
    if [ -n "${listen_pid}" ]; then
        local comm
        comm=$(proc_name "${listen_pid}")
        if [ "${comm}" = "${expected_comm}" ]; then
            log_warn "${service} still listening on ${port} (PID: ${listen_pid}); stopping by port."
            kill "${listen_pid}" 2>/dev/null || true
        else
            log_warn "Port ${port} is occupied by PID ${listen_pid} (comm=${comm}); not killing."
        fi
    fi
}

stop_all() {
    log_info "Stopping all services..."
    for entry in "${SERVICES[@]}"; do
        local service="${entry%%:*}"
        local service_dir="${EXCHANGE_ROOT}/${service}"
        local port="${entry##*:}"

        stop_service "$service" "$port"
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
