# OpenExchange - High-Performance Cryptocurrency Exchange Platform

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-blue?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/PostgreSQL-15-red?style=flat-square&logo=postgresql" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7-red?style=flat-square&logo=redis" alt="Redis">
  <img src="https://img.shields.io/badge/Microservices-Architecture-blue?style=flat-square" alt="Microservices">
</p>

A production-ready, high-performance cryptocurrency exchange platform built with Go, featuring a microservices architecture, memory-based matching engine, and real-time market data.

Keywords: crypto exchange, matching engine, order book, clearing and settlement, market data, Go microservices.

## ⚠️ Important Disclaimer

**FINANCIAL SOFTWARE WARNING**: This software is provided for educational and research purposes only. Exchange software involves complex financial regulations, security requirements, and compliance obligations. **DO NOT use this software for production financial services without:**

- Comprehensive security audits by qualified professionals
- Legal compliance review for your jurisdiction
- Proper regulatory licenses and approvals
- Rigorous testing and load validation
- Compliance with KYC/AML requirements
- Implementation of proper risk management controls

The authors assume no liability for any financial losses or regulatory violations resulting from the use of this software.

## 🚀 Features

### Core Trading
- **Memory Matching Engine**: High-performance order matching with price-time priority
- **Order Types**: LIMIT and MARKET orders with IOC/FOK support
- **Real-time Order Book**: Bid-ask spread management with efficient data structures
- **Trade Execution**: Atomic trade creation with guaranteed consistency

### Market Data
- **REST API**: Depth, trades, and ticker endpoints
- **WebSocket Push**: Real-time order book and trade streams
- **Heartbeat**: Connection health monitoring

### User Management
- **Authentication**: JWT-based auth with bcrypt password hashing
- **API Keys**: Secure API key generation and management
- **RBAC**: Role-based access control (Admin, Operator, Support, Auditor)

### Operations
- **Kill Switch**: Emergency trading halt capability
- **Audit Logging**: Comprehensive activity tracking
- **Configuration**: Dynamic trading pair management

### Infrastructure
- **Microservices**: 8 independent services with gRPC communication
- **Event-Driven**: Redis Streams for async message processing
- **Observability**: OpenTelemetry tracing, Prometheus metrics, structured logging
- **Containerization**: Docker Compose for local development

## 🌍 Multilingual & SEO

- **English README**: [README.md](README.md)
- **中文 README**: [README.zh-CN.md](README.zh-CN.md)
- **Documentation (EN)**: [docs/](docs)
- **Documentation (ZH)**: [交易所项目文档/](交易所项目文档)

## 🏗️ Architecture

```
Client → Gateway(8080) → Order(8081) → Matching(8082)
                              │
                              ↓
                    Redis Streams (events)
                              │
           +------------------+------------------+
           ↓                  ↓                  ↓
      Clearing(8083)    MarketData(8084)    Order Service
           ↓                  ↓
      PostgreSQL         WebSocket Push
```

## 📦 Services

| Service | Port | Status | Description |
|---------|------|--------|-------------|
| `exchange-common` | - | ✅ | Proto definitions, shared utilities, DB schema |
| `exchange-gateway` | 8080 | ✅ | API gateway, signature verification, rate limiting |
| `exchange-user` | 8085 | ✅ | User registration, login, API key management |
| `exchange-order` | 8081 | ✅ | Order creation, cancellation, query |
| `exchange-matching` | 8082 | ✅ | In-memory order book, matching engine |
| `exchange-clearing` | 8083 | ✅ | Fund freezing, settlement, ledger |
| `exchange-marketdata` | 8084/8094 | ✅ | Market data, REST/WebSocket APIs |
| `exchange-admin` | 8087 | 🟡 | Admin operations, RBAC (in progress) |
| `exchange-wallet` | 8086 | 🟡 | Deposits/withdrawals (in progress) |

## 🛠️ Quick Start

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- PostgreSQL 15+ (or use Docker)
- Redis 7+

### Development Setup

```bash
# Clone the repository
git clone https://github.com/tytsxai/exchange-platform.git
cd exchange-platform

# Start infrastructure (PostgreSQL, Redis, Jaeger, Grafana)
docker compose up -d

# Run database migrations
# (see scripts/init-db.sql for schema)

# Start all services
bash exchange-common/scripts/start-all.sh start
```

### Environment Configuration

```bash
# Copy example environment file
cp .env.example .env

# Edit with your configuration
# ⚠️ IMPORTANT: Change all secrets before production use!
```

## 📖 Documentation

- 🌐 Docs Site: https://tytsxai.github.io/exchange-platform/
- [Architecture Overview](docs/architecture.md)
- [API Documentation](docs/api.md)
- [Data Models](docs/data-models.md)
- [Event Specifications](docs/event-model.md)
- [Runbook & Operations](docs/ops/runbook.md)

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
```

## 🔒 Security

See [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## ⚡ Performance Notes

- Single symbol throughput: 10,000+ orders/second
- Matching latency: <100μs (in-memory)
- WebSocket connections: 10,000+ concurrent supported
- Trade confirmation: <50ms end-to-end

## ⚠️ Production Readiness Checklist

Before using in production, ensure:

- [ ] Security audit completed
- [ ] All secrets rotated
- [ ] TLS/HTTPS configured
- [ ] Rate limiting tuned
- [ ] Monitoring/alerting configured
- [ ] Backup/recovery procedures tested
- [ ] Compliance requirements met
- [ ] Load testing passed
- [ ] Disaster recovery plan in place

---

**Remember**: Operating a cryptocurrency exchange requires significant technical expertise, legal compliance, and financial responsibility.
