# Project Roadmap

Development milestones and future roadmap for OpenExchange.

## 📋 Table of Contents

- [Completed Milestones](#completed-milestones)
- [Current Phase](#current-phase)
- [Future Roadmap](#future-roadmap)
- [Contributing](#contributing)

---

## Completed Milestones

### ✅ M0: Infrastructure (Completed)

| Feature | Status | Description |
|---------|--------|-------------|
| Proto Definitions | ✅ Done | Order, Trade, Account, Market, User, Event protos |
| Shared Utilities | ✅ Done | Snowflake ID, Decimal, Signature, Redis, Logger |
| Database Schema | ✅ Done | PostgreSQL schemas per service |
| Docker Compose | ✅ Done | Development environment |

### ✅ M1: Spot Trading (Completed)

| Feature | Status | Description |
|---------|--------|-------------|
| User Service | ✅ Done | Registration, Login, API Keys |
| API Gateway | ✅ Done | Routing, Auth, Rate Limiting |
| Order Service | ✅ Done | Order CRUD, Validation |
| Matching Engine | ✅ Done | In-memory order book |
| Clearing Service | ✅ Done | Settlement, Ledger |
| Market Data | ✅ Done | REST + WebSocket APIs |

---

## Current Phase

### 🟡 M2: Operations & Wallet (In Progress)

#### M2.1 Admin Operations

| Feature | Status | Priority |
|---------|--------|----------|
| RBAC | ✅ Done | High |
| Symbol Management | ✅ Done | High |
| Kill Switch | ✅ Done | High |
| Audit Logging | ✅ Done | High |
| Withdrawal Review | 🔲 Pending | Medium |
| Manual Adjustments | 🔲 Pending | Low |

#### M2.2 Wallet Operations

| Feature | Status | Priority |
|---------|--------|----------|
| Deposit Scanner | ✅ Done | High |
| Withdrawal Processing | ✅ Done | High |
| Address Whitelist | 🔲 Pending | Medium |
| On-Chain Reconciliation | 🔲 Pending | Medium |
| Risk Rules | 🔲 Pending | Medium |

---

## Future Roadmap

### M3: Enhanced Trading (Q1 2026)

#### Trading Features

| Feature | Priority | Complexity |
|---------|----------|------------|
| Margin Trading | High | Complex |
| Leverage Levels | High | Medium |
| Isolated Margin | Medium | Complex |
| Cross Margin | Medium | Complex |

#### Order Types

| Feature | Priority | Status |
|---------|----------|--------|
| Stop Loss | High | Not Started |
| Stop Limit | High | Not Started |
| Trailing Stop | Medium | Not Started |
| OCO (One Cancels Other) | Low | Not Started |

### M4: Advanced Features (Q2 2026)

#### Market Data

| Feature | Priority | Status |
|---------|----------|--------|
| K-Line History | Medium | Not Started |
| Price Alerts | Low | Not Started |
| WebSocket Compression | Low | Not Started |

#### User Features

| Feature | Priority | Status |
|---------|----------|--------|
| Two-Factor Auth | High | Not Started |
| Withdrawal Whitelist | High | Not Started |
| Sub-Accounts | Medium | Not Started |
| API IP Restrictions | Medium | Not Started |

### M5: Scalability (Q3 2026)

#### Performance

| Target | Current | Goal |
|--------|---------|------|
| Orders/sec (single symbol) | 10,000 | 50,000 |
| Matching latency (p99) | 100μs | 50μs |
| Concurrent connections | 10,000 | 100,000 |

#### Infrastructure

| Feature | Priority | Status |
|---------|----------|--------|
| Kubernetes Deployment | High | Not Started |
| Horizontal Scaling | High | Not Started |
| Multi-Region | Medium | Not Started |

### M6: Compliance (Q4 2026)

| Feature | Priority | Status |
|---------|----------|--------|
| KYC Integration | High | Not Started |
| AML Screening | High | Not Started |
| Audit Trails | Medium | Partial |
| Data Retention | Medium | Not Started |

---

## Feature Matrix

### Core Trading

| Feature | Implemented | ETA |
|---------|-------------|-----|
| Limit Orders | ✅ | Done |
| Market Orders | ✅ | Done |
| GTC | ✅ | Done |
| IOC | ✅ | Done |
| FOK | ✅ | Done |
| Post-Only | ✅ | Done |
| Stop Loss | 🔲 | Q1 2026 |
| Stop Limit | 🔲 | Q1 2026 |

### Market Data

| Feature | Implemented | ETA |
|---------|-------------|-----|
| REST Depth | ✅ | Done |
| REST Trades | ✅ | Done |
| REST Ticker | ✅ | Done |
| WS Depth | ✅ | Done |
| WS Trades | ✅ | Done |
| WS Ticker | ✅ | Done |
| K-Lines | 🔲 | Q2 2026 |

### User Management

| Feature | Implemented | ETA |
|---------|-------------|-----|
| Registration | ✅ | Done |
| Login (JWT) | ✅ | Done |
| API Keys | ✅ | Done |
| 2FA | 🔲 | Q2 2026 |
| Sub-Accounts | 🔲 | Q2 2026 |

### Wallet

| Feature | Implemented | ETA |
|---------|-------------|-----|
| Deposits | ✅ | Done |
| Withdrawals | ✅ | Done |
| Address Whitelist | 🔲 | Q2 2026 |
| On-Chain Recon | 🔲 | Q2 2026 |

### Admin

| Feature | Implemented | ETA |
|---------|-------------|-----|
| RBAC | ✅ | Done |
| Symbol Config | ✅ | Done |
| Kill Switch | ✅ | Done |
| Audit Logs | ✅ | Done |
| Withdrawal Review | 🔲 | Q1 2026 |

---

## Development Phases

### Phase 1: Foundation (Completed)
- [x] Microservices architecture
- [x] Core trading engine
- [x] Basic wallet operations

### Phase 2: Production Ready (In Progress)
- [x] Admin operations
- [x] Enhanced security
- [ ] Performance optimization
- [ ] Comprehensive testing

### Phase 3: Feature Rich
- [ ] Margin trading
- [ ] Advanced order types
- [ ] Enhanced user features

### Phase 4: Enterprise
- [ ] Multi-region deployment
- [ ] Compliance features
- [ ] Institutional features

---

## Contributing

### How to Help

We welcome contributions! Areas needing help:

1. **Documentation** - Improve guides and examples
2. **Testing** - Add unit tests, integration tests
3. **Features** - Implement roadmap items
4. **Performance** - Optimize hot paths
5. **Security** - Security audits, improvements

### Getting Started

1. Check [CONTRIBUTING.md](../CONTRIBUTING.md)
2. Pick an issue from GitHub
3. Fork and create a PR

---

## 📖 Related Documentation

- [README](../README.md) - Project overview
- [Architecture](architecture.md) - System design
- [Development](development.md) - Development guide
- [Deployment](deployment.md) - Production deployment
