# Quick Start Guide

Get OpenExchange running in 5 minutes!

## Prerequisites

- Docker & Docker Compose
- Git
- 4GB RAM available

## 🚀 5-Minute Setup

### 1. Clone and Enter

```bash
git clone https://github.com/tytsxai/exchange-platform.git
cd exchange-platform
```

### 2. Start Infrastructure

```bash
docker compose -f exchange-common/docker-compose.yml up -d
```

Wait 30 seconds for services to start.

### 3. Initialize Database

```bash
bash exchange-common/scripts/migrate.sh
```

### 4. Start All Services

```bash
bash exchange-common/scripts/start-all.sh start
```

### 5. Verify

```bash
# Check health
curl -sf http://localhost:8080/health

# Should return:
# {"status":"ok","services":["postgres","redis"]}
```

## ✅ You're Ready!

Services are now running:

| Service | URL | Description |
|---------|-----|-------------|
| Gateway | http://localhost:8080 | Trading API |
| API Docs | http://localhost:8080/docs | Swagger UI |
| Grafana | http://localhost:3000 | Dashboards |
| Jaeger | http://localhost:16686 | Tracing |

## 📝 Your First API Call

### 1. Create a User (Register)

```bash
curl -X POST http://localhost:8080/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'
```

### 2. Get API Key

```bash
curl -X POST http://localhost:8080/v1/apiKeys \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"permissions":["TRADE","READ"]}'
```

### 3. Place Your First Order

```bash
# Sign request with your API key
# See [API Documentation](api.md) for signing details
```

## 🔧 Common Commands

| Command | Description |
|---------|-------------|
| `bash exchange-common/scripts/start-all.sh start` | Start all services |
| `bash exchange-common/scripts/start-all.sh stop` | Stop all services |
| `bash exchange-common/scripts/start-all.sh restart` | Restart all services |
| `docker compose logs -f gateway` | View gateway logs |

## 🐛 Troubleshooting

**Port already in use?**
```bash
# Kill process on port 8080
lsof -ti:8080 | xargs kill -9
```

**Database connection failed?**
```bash
# Restart infrastructure
docker compose -f exchange-common/docker-compose.yml restart postgres redis
```

**Services not starting?**
```bash
# Check logs
docker compose -f exchange-common/docker-compose.yml logs
```

## 📖 Next Steps

1. [Read the Architecture](architecture.md)
2. [Explore the API](api.md)
3. [Set Up Development Environment](development.md)
4. [Deploy to Production](deployment.md)

## ⚠️ Important

This is **development mode** only!

For production use:
- Change all default secrets
- Enable TLS/SSL
- Configure production database
- Review [Security Checklist](deployment.md#security-checklist)
