# Deployment Guide

Deploying OpenExchange to production requires careful planning and configuration.

## 🏗️ Deployment Options

### Option 1: Docker Compose (Development/Small Scale)

Best for development and small deployments.

```bash
# Clone the repository
git clone https://github.com/tytsxai/exchange-platform.git
cd exchange-platform

# Configure environment
cp .env.example .env
# Edit .env with your production values

# Start all services
docker compose -f deploy/prod/docker-compose.yml up -d
```

### Option 2: Kubernetes (Production Recommended)

Best for production deployments with high availability.

**Prerequisites:**
- Kubernetes 1.25+
- Helm 3.0+
- Ingress controller
- External PostgreSQL
- External Redis with clustering

```bash
# Install using Helm
helm repo add openexchange https://tytsxai.github.io/exchange-platform
helm install my-release openexchange/exchange-platform \
  --values values-production.yaml
```

## 📋 Pre-Deployment Checklist

### Infrastructure

- [ ] PostgreSQL 15+ installed (or use managed service)
- [ ] Redis 7+ installed (or use managed service)
- [ ] TLS certificates obtained
- [ ] Domain DNS configured
- [ ] Firewall rules configured

### Security

- [ ] All secrets rotated from defaults
- [ ] API key encryption key generated (32+ characters)
- [ ] JWT secret configured
- [ ] Internal token configured
- [ ] Admin token configured
- [ ] Rate limiting configured

### Database

- [ ] Database created
- [ ] Schemas created
- [ ] Initial migrations applied
- [ ] Backups configured

## 🔧 Configuration

### Environment Variables

#### Core Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_ENV` | Yes | `dev` | Environment (dev/staging/prod) |
| `APP_VERSION` | No | - | Application version tag |
| `LOG_LEVEL` | No | `info` | Logging level (debug/info/warn/error) |

#### Database

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_HOST` | Yes | `localhost` | PostgreSQL host |
| `DB_PORT` | Yes | `5432` | PostgreSQL port |
| `DB_USER` | Yes | - | Database user |
| `DB_PASSWORD` | Yes | - | Database password |
| `DB_NAME` | Yes | - | Database name |
| `DB_SSL_MODE` | Yes | `disable` | SSL mode (require for prod) |
| `DB_MAX_OPEN_CONNS` | No | `25` | Max connections |
| `DB_MAX_IDLE_CONNS` | No | `5` | Idle connections |
| `DB_CONN_MAX_LIFETIME` | No | `5m` | Connection lifetime |

#### Redis

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `REDIS_ADDR` | Yes | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | No | - | Redis password |
| `REDIS_DB` | No | `0` | Redis database |
| `REDIS_TLS` | No | `false` | Enable TLS |

#### Authentication

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `INTERNAL_TOKEN` | Yes | - | Service-to-service auth |
| `AUTH_TOKEN_SECRET` | Yes | - | JWT secret (32+ chars) |
| `AUTH_TOKEN_TTL` | No | `24h` | Token validity |
| `API_KEY_SECRET_KEY` | Yes | - | API key encryption |
| `ADMIN_TOKEN` | Yes | - | Admin endpoints |

#### Gateway

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CORS_ALLOW_ORIGINS` | Yes | - | CORS origins (comma-separated) |
| `TRUSTED_PROXY_CIDRS` | No | - | Trusted proxy IPs |
| `ENABLE_DOCS` | No | `false` | Enable Swagger UI |
| `RATE_LIMIT` | No | `100` | Requests/second |

### Production .env Example

```bash
# Core
APP_ENV=prod
APP_VERSION=v1.0.0

# Database (PostgreSQL with TLS)
DB_HOST=db.example.com
DB_PORT=5432
DB_USER=exchange
DB_PASSWORD=your-secure-password
DB_NAME=exchange
DB_SSL_MODE=require
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=20
DB_CONN_MAX_LIFETIME=10m

# Redis with password and TLS
REDIS_ADDR=redis.example.com:6380
REDIS_PASSWORD=your-redis-password
REDIS_TLS=true

# Authentication (IMPORTANT: Use strong secrets!)
INTERNAL_TOKEN=your-internal-token-min-32-chars
AUTH_TOKEN_SECRET=your-jwt-secret-min-32-chars
AUTH_TOKEN_TTL=24h
API_KEY_SECRET_KEY=your-api-key-encryption-key-min-32chars
ADMIN_TOKEN=your-admin-token-min-32-chars

# Gateway
CORS_ALLOW_ORIGINS=https://your-domain.com
TRUSTED_PROXY_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16
ENABLE_DOCS=false

# Rate Limiting
RATE_LIMIT=100

# Metrics (optional)
METRICS_TOKEN=your-metrics-token
```

## 🚀 Deployment Steps

### 1. Prepare Infrastructure

```bash
# Create network
docker network create exchange-prod-net

# Start PostgreSQL
docker run -d \
  --name exchange-postgres \
  --network exchange-prod-net \
  -e POSTGRES_USER=exchange \
  -e POSTGRES_PASSWORD=your-password \
  -e POSTGRES_DB=exchange \
  -v postgres-data:/var/lib/postgresql/data \
  postgres:15-alpine

# Start Redis
docker run -d \
  --name exchange-redis \
  --network exchange-prod-net \
  -v redis-data:/data \
  redis:7-alpine \
  redis-server --requirepass your-redis-password
```

### 2. Initialize Database

```bash
# Apply migrations
export DB_URL="postgres://exchange:your-password@localhost:5432/exchange?sslmode=require"
bash exchange-common/scripts/migrate.sh
```

### 3. Build and Deploy Services

```bash
# Build all services
docker compose -f deploy/prod/docker-compose.yml build

# Deploy
docker compose -f deploy/prod/docker-compose.yml \
  --env-file deploy/prod/prod.env \
  up -d
```

### 4. Verify Deployment

```bash
# Check service health
curl -sf http://localhost:8080/health
curl -sf http://localhost:8080/ready

# Check logs
docker compose -f deploy/prod/docker-compose.yml logs -f gateway
```

## 🔄 Rolling Updates

### Using Docker Compose

```bash
# Pull latest images
docker compose -f deploy/prod/docker-compose.yml pull

# Rolling restart
docker compose -f deploy/prod/docker-compose.yml up -d
```

### Zero-Downtime Update

```bash
# Start new containers alongside old ones
docker compose -f deploy/prod/docker-compose.yml up -d --no-recreate

# Wait for new containers to be healthy
until curl -sf http://localhost:8080/ready; do
    echo "Waiting for new version..."
    sleep 5
done

# Stop old containers
docker compose -f deploy/prod/docker-compose.yml stop old-gateway
docker compose -f deploy/prod/docker-compose.yml rm old-gateway
```

## ⏪ Rollback Procedure

### Docker Compose Rollback

```bash
# View previous versions
docker compose -f deploy/prod/docker-compose.yml ps

# Rollback to previous image
docker tag exchange-platform/gateway:previous exchange-platform/gateway:latest
docker tag exchange-platform/order:previous exchange-platform/order:latest
# ... repeat for all services

# Restart with previous version
docker compose -f deploy/prod/docker-compose.yml up -d
```

### Database Rollback

```bash
# If migration included database changes
# Restore from backup
pg_restore -h localhost -U exchange -d exchange backup.dump
```

## 📊 Monitoring

### Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `/live` | Liveness probe (am I running?) |
| `/ready` | Readiness probe (am I ready to serve?) |
| `/health` | Detailed health status |

```bash
# Check all services
curl -sf http://localhost:8080/health | jq '.'
```

### Metrics

Access Prometheus metrics at `/metrics` (protected by `METRICS_TOKEN`).

**Key Metrics:**
- `http_requests_total` - Request count
- `http_request_duration_seconds` - Latency histogram
- `orders_total` - Order throughput
- `trades_total` - Trade count
- `matching_latency_seconds` - Matching latency
- `orderbook_depth` - Order book depth

### Alerting

Configure alerts in `deploy/prod/alerts.yml`:

```yaml
groups:
  - name: exchange-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
```

## 🔒 Security Checklist

- [ ] All default secrets changed
- [ ] TLS enabled on all endpoints
- [ ] Database SSL required
- [ ] Redis password configured
- [ ] API docs disabled in production
- [ ] Metrics endpoint protected
- [ ] Rate limiting configured
- [ ] IP whitelist configured (optional)
- [ ] Audit logging enabled
- [ ] Regular secret rotation scheduled

## 📈 Performance Tuning

### Database

```sql
-- Create indexes for common queries
CREATE INDEX idx_orders_symbol_time ON exchange_order.orders(symbol, created_at DESC);
CREATE INDEX idx_trades_symbol_time ON exchange_order.trades(symbol, created_at DESC);
CREATE INDEX idx_ledger_user_time ON exchange_clearing.ledger(user_id, created_at DESC);
```

### Redis

```bash
# Optimize memory usage
redis-cli CONFIG SET maxmemory 2gb
redis-cli CONFIG SET maxmemory-policy allkeys-lru
```

### Application

```bash
# Adjust connection pool based on load
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=25
```

## 🆘 Troubleshooting

### Service Won't Start

```bash
# Check logs
docker compose -f deploy/prod/docker-compose.yml logs service-name

# Check health
docker inspect --format='{{.State.Health.Status}}' container-name
```

### Database Connection Failed

```bash
# Test connection
PGPASSWORD=your-password psql -h host -U exchange -d exchange

# Check SSL
psql "sslmode=require host=host user=exchange dbname=exchange"
```

### Redis Connection Issues

```bash
# Test connection
redis-cli -h redis-host -p 6380 -a your-password ping
```
