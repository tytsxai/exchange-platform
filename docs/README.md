# OpenExchange Documentation

Comprehensive documentation for the OpenExchange cryptocurrency exchange platform.

## 📚 Documentation Structure

### Getting Started
- [Quick Start](quickstart.md) - Get trading services running in 5 minutes
- [Architecture](architecture.md) - System architecture overview

### Core Concepts
- [Trading Flow](trading-flow.md) - Order lifecycle from creation to settlement
- [Data Models](data-models.md) - Proto definitions and database schemas
- [Glossary](glossary.md) - Exchange terminology

### Development
- [Development Guide](development.md) - Setting up local development environment
- [Code Style](code-style.md) - Coding conventions and best practices
- [Testing](testing.md) - Writing and running tests

### API Reference
- [API Overview](api.md) - API architecture and authentication
- [Gateway API](api.md) - Trading, market data, and account endpoints

### Operations
- [Deployment](deployment.md) - Production deployment guide
- [Configuration](configuration.md) - Environment variables and config options
- [Monitoring](monitoring.md) - Observability, metrics, and tracing
- [Runbook](ops/runbook.md) - Production operations handbook
- [Backup & Recovery](ops/backup-restore.md) - Data backup procedures
- [Production Readiness](ops/production-ready.md) - Pre-launch checklist

### Reference
- [Error Codes](ops/production-ready.md#error-codes) - Error handling
- [Withdraw State Machine](ops/withdraw-state-machine.md) - Withdrawal flow

## 🚀 Quick Links

| Topic | Description |
|-------|-------------|
| [GitHub Repository](https://github.com/tytsxai/exchange-platform) | Main repository |
| [Architecture Diagram](architecture.md) | System design overview |
| [API Endpoints](api.md) | REST API reference |
| [WebSocket Protocol](api.md#websocket) | Real-time market data |

## Architecture Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│   Gateway   │────▶│   Order    │
│             │     │   (8080)    │     │  Service   │
└─────────────┘     └─────────────┘     │  (8081)    │
                                        └─────┬───────┘
                                              │
                                              ▼
                                        ┌─────────────┐
                                        │  Matching   │
                                        │  Engine     │
                                        │  (8082)     │
                                        └─────┬───────┘
                                              │
           ┌────────────────┬────────────────┼────────────────┐
           ▼                ▼                ▼                ▼
    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
    │  Clearing   │  │Market Data  │  │Order Update │  │   Wallet    │
    │  (8083)     │  │  (8084)     │  │             │  │  (8086)     │
    └──────┬──────┘  └─────────────┘  └─────────────┘  └─────────────┘
           │
           ▼
    ┌─────────────┐
    │PostgreSQL   │
    └─────────────┘
```

## Service Ports

| Service | Port | Protocol | Description |
|---------|------|----------|-------------|
| Gateway | 8080 | HTTP/WS | API gateway, trading, market data |
| Order | 8081 | gRPC | Order management |
| Matching | 8082 | gRPC | Matching engine |
| Clearing | 8083 | gRPC | Settlement |
| MarketData | 8084/8094 | HTTP/WS | Market data feed |
| Wallet | 8086 | HTTP | Deposits/withdrawals |
| Admin | 8087 | HTTP | Admin operations |

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines.

## License

This project is licensed under the MIT License. See [LICENSE](../LICENSE).
