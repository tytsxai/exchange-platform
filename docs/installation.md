# Installation Guide

Complete installation guide for OpenExchange development and production environments.

## 📋 Table of Contents

- [Prerequisites](#prerequisites)
- [Development Setup](#development-setup)
- [Production Deployment](#production-deployment)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 4 cores | 8+ cores |
| Memory | 8 GB | 16+ GB |
| Storage | 50 GB SSD | 200+ GB SSD |
| Network | 100 Mbps | 1 Gbps |

### Required Software

| Software | Version | Required For |
|----------|---------|--------------|
| Go | 1.25+ | Development, Build |
| Docker | Latest | Container Runtime |
| Docker Compose | Latest | Local Development |
| PostgreSQL | 15+ | Database |
| Redis | 7+ | Caching, Streams |
| Git | Latest | Version Control |

### Optional Software

| Software | Version | Purpose |
|----------|---------|----------|
| Kubernetes | 1.25+ | Production Orchestration |
| Helm | 3.0+ | Kubernetes Package Manager |
| Prometheus | 2.0+ | Metrics |
| Grafana | 10.0+ | Visualization |
| Jaeger | 1.0+ | Tracing |

---

## Development Setup

### 1. Clone Repository

```bash
# Clone the repository
git clone https://github.com/tytsxai/exchange-platform.git
cd exchange-platform
```

### 2. Install Dependencies

```bash
# Download Go dependencies
go mod download

# Install development tools
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 3. Generate Proto Files

```bash
# Generate Go code from proto definitions
chmod +x exchange-common/scripts/gen-proto.sh
./exchange-common/scripts/gen-proto.sh
```

### 4. Start Infrastructure

```bash
# Start PostgreSQL, Redis, and monitoring tools
docker compose -f exchange-common/docker-compose.yml up -d

# Verify services
docker compose -f exchange-common/docker-compose.yml ps
```

#### Service Ports

| Service | Port | Credentials |
|---------|------|-------------|
| PostgreSQL | 5436 | exchange/exchange123 |
| Redis | 6380 | no password |
| Prometheus | 9090 | - |
| Grafana | 3000 | admin/admin123 |
| Jaeger | 16686 | - |

### 5. Initialize Database

```bash
# Run database migrations
export DATABASE_URL="postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"
bash exchange-common/scripts/migrate.sh
```

### 6. Start All Services

```bash
# Start all exchange services
bash exchange-common/scripts/start-all.sh start

# Or start individual services
cd exchange-gateway && go run ./cmd/gateway  # Port 8080
cd exchange-order && go run ./cmd/order      # Port 8081
cd exchange-matching && go run ./cmd/matching # Port 8082
cd exchange-clearing && go run ./cmd/clearing # Port 8083
cd exchange-marketdata && go run ./cmd/marketdata # Ports 8084/8094
cd exchange-user && go run ./cmd/user        # Port 8085
cd exchange-wallet && go run ./cmd/wallet   # Port 8086
cd exchange-admin && go run ./cmd/admin     # Port 8087
```

### 7. Verify Installation

```bash
# Check health
curl -sf http://localhost:8080/health

# Expected response:
# {"status":"ok","services":{"postgres":"ok","redis":"ok"}}

# Check ready
curl -sf http://localhost:8080/ready
# Expected response:
# {"status":"ready"}
```

---

## Production Deployment

### 1. Prepare Environment

```bash
# Create production environment file
cp .env.example .env.production
```

#### Production Environment Variables

```bash
# .env.production

# Core
APP_ENV=prod
APP_VERSION=v1.0.0

# Database (with TLS)
DB_HOST=your-db-host
DB_PORT=5432
DB_USER=exchange
DB_PASSWORD=your-secure-password
DB_NAME=exchange
DB_SSL_MODE=require
NS=100
DB_MAX_OPEN_CONDB_MAX_IDLE_CONNS=20
DB_CONN_MAX_LIFETIME=10m

# Redis (with password and TLS)
REDIS_ADDR=your-redis-host:6380
REDIS_PASSWORD=your-redis-password
REDIS_TLS=true

# Authentication (USE STRONG SECRETS!)
INTERNAL_TOKEN=your-internal-token-min-32-chars
AUTH_TOKEN_SECRET=your-jwt-secret-min-32-chars
AUTH_TOKEN_TTL=24h
API_KEY_SECRET_KEY=your-api-key-encryption-key-min-32-chars
ADMIN_TOKEN=your-admin-token-min-32-chars

# Gateway
CORS_ALLOW_ORIGINS=https://your-domain.com
ENABLE_DOCS=false
```

### 2. Build Images

```bash
# Build all service images
docker compose -f deploy/prod/docker-compose.yml build

# Or build individual services
docker build -t exchange-gateway:prod ./exchange-gateway
docker build -t exchange-order:prod ./exchange-order
# ... repeat for all services
```

### 3. Configure Infrastructure

```bash
# Create Docker network
docker network create exchange-prod-net

# Start PostgreSQL (recommended: use managed service)
docker run -d \
  --name exchange-postgres \
  --network exchange-prod-net \
  -e POSTGRES_USER=exchange \
  -e POSTGRES_PASSWORD=your-password \
  -e POSTGRES_DB=exchange \
  -v postgres-data:/var/lib/postgresql/data \
  postgres:15-alpine

# Start Redis (recommended: use managed service)
docker run -d \
  --name exchange-redis \
  --network exchange-prod-net \
  -v redis-data:/data \
  redis:7-alpine \
  redis-server --requirepass your-password --tls-port 6380
```

### 4. Initialize Database

```bash
# Apply migrations
export DATABASE_URL="postgres://exchange:your-password@your-db-host:5432/exchange?sslmode=require"
bash exchange-common/scripts/migrate.sh
```

### 5. Deploy Services

```bash
# Deploy with Docker Compose
docker compose -f deploy/prod/docker-compose.yml \
  --env-file deploy/prod/prod.env \
  up -d

# Check status
docker compose -f deploy/prod/docker-compose.yml ps
```

### 6. Verify Deployment

```bash
# Health check all services
for service in gateway order matching clearing marketdata wallet admin; do
  echo "Checking $service..."
  curl -sf "http://localhost:8080/health" || echo "$service health check failed"
done
```

---

## Docker Deployment (Alternative)

### Single Docker Compose File

```yaml
# deploy/docker-compose.yml
version: '3.8'

services:
  gateway:
    build: ./exchange-gateway
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://exchange:exchange123@postgres:5432/exchange?sslmode=disable
      - REDIS_ADDR=redis:6380
    depends_on:
      - postgres
      - redis
    networks:
      - exchange-net

  order:
    build: ./exchange-order
    depends_on:
      - postgres
      - redis
    networks:
      - exchange-net

  matching:
    build: ./exchange-matching
    depends_on:
      - redis
    networks:
      - exchange-net

  clearing:
    build: ./exchange-clearing
    depends_on:
      - postgres
      - redis
    networks:
      - exchange-net

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: exchange
      POSTGRES_PASSWORD: exchange123
      POSTGRES_DB: exchange
    volumes:
      - postgres-data:/var/lib/postgresql/data
    networks:
      - exchange-net

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
    networks:
      - exchange-net

networks:
  exchange-net:
    driver: bridge

volumes:
  postgres-data:
  redis-data:
```

```bash
# Deploy
docker compose -f deploy/docker-compose.yml up -d
```

---

## Kubernetes Deployment

### Helm Chart Values

```yaml
# values-production.yaml

global:
  imageRegistry: docker.io
  imagePullSecrets:
    - registry-secret

replicaCount:
  gateway: 3
  order: 3
  matching: 3
  clearing: 3
  marketdata: 2
  wallet: 2
  admin: 2

resources:
  gateway:
    requests:
      cpu: 1000m
      memory: 512Mi
    limits:
      cpu: 2000m
      memory: 1Gi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: api.your-domain.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: api-tls
      hosts:
        - api.your-domain.com
```

### Deploy with Helm

```bash
# Add Helm repository
helm repo add openexchange https://tytsxai.github.io/exchange-platform

# Install
helm install my-release openexchange/exchange-platform \
  --values values-production.yaml \
  --namespace exchange \
  --create-namespace
```

---

## Verification

### API Health Check

```bash
# Check all endpoints
ENDPOINTS=(
    "http://localhost:8080/health"
    "http://localhost:8080/ready"
    "http://localhost:8080/live"
)

for endpoint in "${ENDPOINTS[@]}"; do
    echo "Checking: $endpoint"
    curl -sf "$endpoint" | jq '.'
done
```

### Integration Test

```bash
# Run integration tests
go test ./... -tags=integration
```

### Performance Check

```bash
# Run load test
hey -n 10000 -c 100 http://localhost:8080/v1/depth?symbol=BTC_USDT
```

---

## Troubleshooting

### Common Issues

#### Database Connection Failed

```bash
# Check PostgreSQL status
docker compose -f exchange-common/docker-compose.yml logs postgres

# Test connection
psql -h localhost -p 5436 -U exchange -d exchange
```

#### Redis Connection Failed

```bash
# Check Redis status
docker compose -f exchange-common/docker-compose.yml logs redis

# Test connection
redis-cli -p 6380 ping
```

#### Service Won't Start

```bash
# Check logs
docker compose -f exchange-common/docker-compose.yml logs -f <service-name>

# Check health status
docker inspect --format='{{.State.Health.Status}}' <container-name>
```

#### Port Already in Use

```bash
# Find process using port
lsof -i :8080

# Kill process
kill -9 <PID>
```

### Reset Environment

```bash
# Stop all services
bash exchange-common/scripts/start-all.sh stop

# Reset database
dropdb -h localhost -p 5436 -U exchange exchange
createdb -h localhost -p 5436 -U exchange exchange
bash exchange-common/scripts/migrate.sh

# Restart services
bash exchange-common/scripts/start-all.sh start
```

---

## 📖 Related Documentation

- [Quick Start](quickstart.md) - 5-minute quick start guide
- [Development](development.md) - Development environment setup
- [Deployment](deployment.md) - Production deployment guide
- [Configuration](configuration.md) - Configuration reference
