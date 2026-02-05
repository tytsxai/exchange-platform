# Development Guide

This guide covers setting up a local development environment for OpenExchange.

## 🛠️ Prerequisites

| Tool | Version | Required |
|------|---------|----------|
| Go | 1.25+ | Yes |
| Docker | Latest | Yes |
| Git | Latest | Yes |
| Make | Latest | Optional |

## 📦 Installation

### 1. Clone the Repository

```bash
git clone https://github.com/tytsxai/exchange-platform.git
cd exchange-platform
```

### 2. Start Development Infrastructure

```bash
# Start PostgreSQL, Redis, and observability tools
docker compose -f exchange-common/docker-compose.yml up -d
```

**Services Started:**

| Service | Port | Credentials |
|---------|------|-------------|
| PostgreSQL | 5436 | exchange/exchange123 |
| Redis | 6380 | no password |
| Prometheus | 9090 | - |
| Grafana | 3000 | admin/admin123 |
| Jaeger | 16686 | - |

### 3. Initialize Database

```bash
# Create schemas and tables
export DATABASE_URL="postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"
bash exchange-common/scripts/migrate.sh
```

### 4. Install Dependencies

```bash
# Download Go dependencies
go mod download

# Generate protobuf code
chmod +x exchange-common/scripts/gen-proto.sh
./exchange-common/scripts/gen-proto.sh
```

## 🏃 Running Services

### Start All Services

```bash
# Using the convenience script
bash exchange-common/scripts/start-all.sh start
```

### Start Individual Service

```bash
cd exchange-gateway
go run ./cmd/gateway

# Or with custom config
HTTP_PORT=8080 go run ./cmd/gateway
```

### Service Ports

| Service | Port | Command |
|---------|------|---------|
| Gateway | 8080 | `cd exchange-gateway && go run ./cmd/gateway` |
| User | 8085 | `cd exchange-user && go run ./cmd/user` |
| Order | 8081 | `cd exchange-order && go run ./cmd/order` |
| Matching | 8082 | `cd exchange-matching && go run ./cmd/matching` |
| Clearing | 8083 | `cd exchange-clearing && go run ./cmd/clearing` |
| MarketData | 8084/8094 | `cd exchange-marketdata && go run ./cmd/marketdata` |
| Admin | 8087 | `cd exchange-admin && go run ./cmd/admin` |
| Wallet | 8086 | `cd exchange-wallet && go run ./cmd/wallet` |

## 🧪 Testing

### Run All Tests

```bash
go test ./...
```

### Run Tests for Specific Module

```bash
go test ./exchange-order/...
go test ./exchange-matching/...
```

### Run with Coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Race Detection

```bash
go test -race ./...
```

## 📝 Project Structure

```
exchange-platform/
├── exchange-common/          # Shared code (proto, utilities)
├── exchange-gateway/         # API gateway
├── exchange-user/           # User management
├── exchange-order/          # Order handling
├── exchange-matching/       # Matching engine
├── exchange-clearing/       # Settlement
├── exchange-marketdata/     # Market data
├── exchange-admin/          # Admin operations
├── exchange-wallet/         # Wallet operations
├── deploy/                   # Deployment configs
├── scripts/                  # Utility scripts
└── docs/                     # Documentation
```

## 🔧 Development Workflow

### 1. Create a Feature Branch

```bash
git checkout -b feature/your-feature-name
```

### 2. Make Changes

Follow the coding standards in [Code Style](code-style.md).

### 3. Test Your Changes

```bash
# Run unit tests
go test ./...

# Run linter
golangci-lint run
```

### 4. Commit Changes

```bash
git add .
git commit -m "feat: Add new feature description"
```

### 5. Push and Create PR

```bash
git push origin feature/your-feature-name
# Create PR on GitHub
```

## 🐛 Debugging

### Enable Debug Logging

```bash
LOG_LEVEL=debug go run ./cmd/gateway
```

### View Logs

```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f gateway
```

### Trace Requests

1. Open Jaeger UI at http://localhost:16686
2. Search for your service
3. Click on a trace to view details

## 📚 Useful Commands

### Database Operations

```bash
# Connect to PostgreSQL
psql -h localhost -p 5436 -U exchange -d exchange

# Connect to Redis
redis-cli -p 6380
```

### Reset Development Environment

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

## 🐳 Docker Development

### Build Service Image

```bash
docker build -t exchange-gateway:dev ./exchange-gateway
```

### Run in Docker

```bash
docker run -it --rm \
  -p 8080:8080 \
  --network exchange-platform_default \
  exchange-gateway:dev
```

## 🔒 Environment Variables

Create a `.env` file for local development:

```bash
# Copy example
cp .env.example .env

# Edit with your settings
# All defaults are suitable for local development
```

## 📖 Additional Resources

- [Architecture](architecture.md)
- [API Documentation](api.md)
- [Testing Guide](testing.md)
- [Code Style](code-style.md)
