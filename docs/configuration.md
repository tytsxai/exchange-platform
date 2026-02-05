# Configuration Reference

Complete reference for all configuration options in OpenExchange.

## Environment Variables

### Common Settings

| Variable | Default | Description |
|-----------|---------|-------------|
| `APP_ENV` | `dev` | Environment: dev/staging/prod |
| `APP_VERSION` | - | Application version |
| `LOG_LEVEL` | `info` | Logging level: debug/info/warn/error |
| `HTTP_PORT` | service-specific | HTTP server port |
| `GRPC_PORT` | service-specific | gRPC server port |

### Database Configuration

```bash
# Connection string format
DATABASE_URL=postgres://user:password@host:port/dbname?sslmode=mode

# Individual parameters
DB_HOST=localhost
DB_PORT=5432
DB_USER=exchange
DB_PASSWORD=exchange123
DB_NAME=exchange
DB_SSL_MODE=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=5m
DB_MAX_CONN_LIFETIME=30m
```

### Redis Configuration

```bash
REDIS_ADDR=localhost:6380
REDIS_PASSWORD=
REDIS_DB=0
REDIS_TLS=false
REDIS_POOL_SIZE=10
REDIS_MIN_IDLE_CONS=5
```

### Authentication

```bash
# Internal service communication
INTERNAL_TOKEN=dev-internal-token

# JWT settings
AUTH_TOKEN_SECRET=your-jwt-secret-min-32-chars
AUTH_TOKEN_TTL=24h
AUTH_REFRESH_TOKEN_TTL=168h

# API Key encryption
API_KEY_SECRET_KEY=your-encryption-key-min-32-chars

# Admin endpoints
ADMIN_TOKEN=your-admin-token
```

### Gateway Service

```bash
# HTTP Server
HTTP_PORT=8080
WS_PORT=8090

# CORS
CORS_ALLOW_ORIGINS=*
CORS_ALLOW_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOW_HEADERS=*

# Rate Limiting
RATE_LIMIT=100
RATE_LIMIT_BURST=200
IP_RATE_LIMIT=1000

# Proxy
TRUSTED_PROXY_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16

# Documentation
ENABLE_DOCS=true
ALLOW_DOCS_IN_NONDEV=false
```

### Matching Engine

```bash
# Matching
MATCHING_ORDER_DEDUP_TTL=24h
MATCHING_SYMBOLS=BTC_USDT,ETH_USDT

# Performance
MATCHING_MAX_ORDERS_PER_SYMBOL=1000000
MATCHING_BATCH_SIZE=100
```

### Wallet Service

```bash
# Scanner
DEPOSIT_SCANNER_ENABLED=false
DEPOSIT_SCANNER_INTERVAL=1m
DEPOSIT_MIN_CONFIRMATIONS=6

# Withdrawals
WITHDRAWAL_FEE_BTC=0.0005
WITHDRAWAL_FEE_ETH=0.005
WITHDRAWAL_MIN_AMOUNT=0.001
WITHDRAWAL_MAX_AMOUNT=1000
```

### Monitoring

```bash
# Metrics
METRICS_ENABLED=true
METRICS_PORT=9090
METRICS_TOKEN=

# Tracing
TRACING_ENABLED=true
TRACING_SAMPLE_RATE=0.1
JAEGER_AGENT_HOST=localhost
JAEGER_AGENT_PORT=6831
```

### Logging

```bash
# Output
LOG_FORMAT=json
LOG_OUTPUT=stdout

# Level by component
LOG_LEVEL_GATEWAY=info
LOG_LEVEL_ORDER=info
LOG_LEVEL_MATCHING=info
```

## Command-Line Flags

### Service Start

```bash
# Gateway
./gateway --help
  --config string     Config file path
  --port int          HTTP port (default 8080)
  --log-level string  Log level (default "info")

# Order Service
./order --help
  --config string     Config file path
  --port int          gRPC port (default 8081)
  --log-level string  Log level (default "info")

# Matching Engine
./matching --help
  --config string     Config file path
  --port int          gRPC port (default 8082)
  --symbols string    Trading symbols (default "BTC_USDT,ETH_USDT")
```

## Configuration File (YAML)

```yaml
# config.yaml example
app:
  name: exchange-gateway
  version: 1.0.0
  env: dev

database:
  host: localhost
  port: 5432
  username: exchange
  password: exchange123
  name: exchange
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m

redis:
  addr: localhost:6380
  password: ""
  db: 0

server:
  http_port: 8080
  grpc_port: 9090
  read_timeout: 30s
  write_timeout: 30s

auth:
  internal_token: ${INTERNAL_TOKEN}
  jwt_secret: ${AUTH_TOKEN_SECRET}
  jwt_ttl: 24h

rate_limit:
  requests_per_second: 100
  burst: 200

logging:
  level: info
  format: json
```

## Service-Specific Ports

| Service | HTTP | gRPC | WebSocket | Description |
|---------|------|------|-----------|-------------|
| gateway | 8080 | - | 8090 | API gateway |
| user | - | 8085 | - | User service |
| order | - | 8081 | - | Order service |
| matching | - | 8082 | - | Matching engine |
| clearing | - | 8083 | - | Settlement |
| marketdata | 8084 | - | 8094 | Market data |
| wallet | 8086 | - | - | Wallet |
| admin | 8087 | - | - | Admin |

## 🔐 Secret Management

### Development

```bash
# Use .env file
cp .env.example .env
# Edit secrets
```

### Production

Use a secrets manager:

```bash
# HashiCorp Vault
export DATABASE_URL=$(vault kv get -field=url secret/exchange/database)

# AWS Secrets Manager
export AUTH_TOKEN_SECRET=$(aws secretsmanager get-secret-value --secret-id exchange/jwt --query SecretString --output text | jq -r .jwt_secret)
```

## ⚠️ Security Considerations

1. **Never commit secrets** to version control
2. **Use strong secrets** (32+ characters, random)
3. **Rotate secrets** regularly in production
4. **Use TLS** in production (DB, Redis, API)
5. **Restrict access** to configuration files
