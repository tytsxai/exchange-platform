# OpenExchange - 高性能加密货币交易所平台

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-blue?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/PostgreSQL-15-red?style=flat-square&logo=postgresql" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7-red?style=flat-square&logo=redis" alt="Redis">
  <img src="https://img.shields.io/badge/Microservices-Architecture-blue?style=flat-square" alt="Microservices">
</p>

这是一个面向生产实践的高性能数字资产交易所后端，采用 Go + 微服务架构，包含内存撮合引擎、清算服务和实时行情推送。

关键词：交易所系统、撮合引擎、订单簿、清结算、行情系统、Go 微服务、crypto exchange、matching engine。

## ⚠️ 重要声明

**金融软件警告**：本项目仅用于学习与研究。交易所系统涉及复杂的监管合规、安全体系和风险控制。**请勿在未完成以下工作前直接用于真实金融业务：**

- 通过专业第三方安全审计
- 完成所在司法辖区法律合规评估
- 取得必要牌照和监管许可
- 完成严格的功能、压力和容灾验证
- 满足 KYC/AML 与风控要求

作者不对任何资金损失、合规风险或其他损害承担责任。

## 🚀 核心特性

### 交易核心
- **内存撮合引擎**：价格优先、时间优先撮合
- **订单类型**：LIMIT / MARKET，支持 IOC / FOK
- **订单簿**：高性能买卖盘维护
- **成交执行**：保证一致性的原子成交流程

### 行情系统
- **REST API**：深度、成交、Ticker 查询
- **WebSocket 推送**：实时深度与逐笔成交
- **心跳机制**：连接健康探测

### 用户与权限
- **鉴权体系**：JWT + bcrypt
- **API Key**：安全创建与管理
- **RBAC**：Admin / Operator / Support / Auditor

### 运营能力
- **Kill Switch**：紧急停盘开关
- **审计日志**：关键行为可追踪
- **配置中心化**：交易对动态管理

### 基础设施
- **微服务架构**：8 个独立服务，gRPC 通信
- **事件驱动**：Redis Streams 异步处理
- **可观测性**：OpenTelemetry + Prometheus + 结构化日志
- **容器化部署**：Docker Compose 本地开发

## 🌍 多语言与传播

- **English README**：[README.md](README.md)
- **中文 README**：[README.zh-CN.md](README.zh-CN.md)
- **英文文档**：[docs/](docs)
- **中文文档**：[交易所项目文档/](交易所项目文档)

## 🏗️ 架构

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

## 📦 服务清单

| 服务 | 端口 | 状态 | 说明 |
|------|------|------|------|
| `exchange-common` | - | ✅ | 协议定义、公共工具、数据库模型 |
| `exchange-gateway` | 8080 | ✅ | API 网关、签名验证、限流 |
| `exchange-user` | 8085 | ✅ | 用户注册、登录、API Key |
| `exchange-order` | 8081 | ✅ | 下单、撤单、查询 |
| `exchange-matching` | 8082 | ✅ | 内存订单簿、撮合引擎 |
| `exchange-clearing` | 8083 | ✅ | 资金冻结、清算、账本 |
| `exchange-marketdata` | 8084/8094 | ✅ | 行情 REST/WebSocket |
| `exchange-admin` | 8087 | 🟡 | 管理后台、RBAC（开发中） |
| `exchange-wallet` | 8086 | 🟡 | 充提系统（开发中） |

## 🛠️ 快速开始

### 环境要求

- Go 1.25+
- Docker & Docker Compose
- PostgreSQL 15+（或使用 Docker）
- Redis 7+

### 本地开发

```bash
# 克隆仓库
git clone https://github.com/tytsxai/exchange-platform.git
cd exchange-platform

# 启动基础设施（PostgreSQL、Redis、Jaeger、Grafana）
docker compose up -d

# 启动全部服务
bash exchange-common/scripts/start-all.sh start
```

### 环境变量

```bash
# 复制示例配置
cp .env.example .env

# 按需修改
# ⚠️ 生产环境务必更换所有密钥
```

## 📖 文档

- 🌐 文档站点：https://tytsxai.github.io/exchange-platform/
- [架构总览](docs/architecture.md)
- [API 文档](docs/api.md)
- [数据模型](docs/data-models.md)
- [事件规范](docs/event-model.md)
- [运维 Runbook](docs/ops/runbook.md)

## 🧪 测试

```bash
# 运行全部测试
go test ./...

# 覆盖率
go test -coverprofile=coverage.out ./...
```

## 🔒 安全

安全报告流程见 [SECURITY.md](SECURITY.md)。

## 📄 许可证

本项目基于 MIT License 开源，详见 [LICENSE](LICENSE)。

## 🤝 贡献

欢迎贡献！请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## ⚡ 性能说明

- 单交易对吞吐：10,000+ orders/s
- 撮合延迟：<100μs（内存撮合）
- WebSocket 连接：10,000+ 并发
- 成交通知：端到端 <50ms

## ⚠️ 生产可用检查清单

上线前请确保：

- [ ] 完成安全审计
- [ ] 完成密钥轮换
- [ ] 启用 TLS/HTTPS
- [ ] 完成限流调优
- [ ] 配置监控告警
- [ ] 验证备份与恢复
- [ ] 满足合规要求
- [ ] 完成压力测试
- [ ] 具备容灾预案

---

**提示**：运营交易所是高风险、高门槛系统工程，请在合规与安全前提下审慎推进。
