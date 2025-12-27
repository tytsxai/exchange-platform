# 交易所开发计划

> 技术栈：Go + PostgreSQL + Redis Streams + 微服务多仓库
> 参考文档：`交易所项目文档/`

## 当前进度

| 里程碑 | 状态 | 说明 |
|--------|------|------|
| M0 基础设施 | ✅ 完成 | Proto、共享工具、Docker Compose |
| M1 现货交易 | ✅ 完成 | 6 个核心服务全部编译通过 |
| M2 行情服务 | ✅ 完成 | REST + WebSocket 推送 |
| M3 运营后台 | 🟡 部分完成 | 已有基础服务与接口；RBAC/审计等持续补齐 |
| M4 钱包出入金 | 🟡 部分完成 | 已有基础服务与接口；链上监听/风控/对账持续补齐 |
| M5 合规安全 | 🔲 待开发 | KYC/AML 预留 |

---

## 仓库清单

| 仓库 | 端口 | 状态 | 说明 |
|------|------|------|------|
| `exchange-common` | - | ✅ | Proto/共享工具/DB Schema |
| `exchange-gateway` | 8080 | ✅ | API 网关、签名验证、限流 |
| `exchange-user` | 8085 | ✅ | 用户注册/登录、API Key |
| `exchange-order` | 8081 | ✅ | 下单/撤单、订单查询 |
| `exchange-matching` | 8082 | ✅ | 内存订单簿、撮合引擎 |
| `exchange-clearing` | 8083 | ✅ | 资金冻结/解冻、账本 |
| `exchange-marketdata` | 8084/8094(WS) | ✅ | 盘口/成交/Ticker/WS推送 |
| `exchange-admin` | 8087 | 🟡 | 运营后台（基础能力已实现，持续完善） |
| `exchange-wallet` | 8086 | 🟡 | 钱包/出入金（基础能力已实现，持续完善） |

---

## M0: 基础设施与共享层 ✅

### M0.1 exchange-common 仓库
- [x] 项目初始化（Go module、目录结构）
- [x] Proto 定义
  - [x] `order.proto`（Order、OrderStatus、Side、Type、TIF）
  - [x] `trade.proto`（Trade、TradeCreated 事件）
  - [x] `account.proto`（Balance、LedgerEntry）
  - [x] `market.proto`（SymbolConfig、Ticker、Depth）
  - [x] `user.proto`（User、ApiKey）
  - [x] `event.proto`（统一事件封装）
- [x] 共享工具
  - [x] 雪花 ID 生成器 (`pkg/snowflake`)
  - [x] 精度计算工具 (`pkg/decimal`)
  - [x] 签名验证工具 (`pkg/signature`)
  - [x] Redis Streams 封装 (`pkg/redis`)
  - [x] 错误码定义 (`pkg/errors`)
- [x] 数据库 Schema (`scripts/init-db.sql`)

### M0.2 基础设施
- [x] Docker Compose 开发环境
  - [x] PostgreSQL
  - [x] Redis（Streams）
  - [x] Prometheus + Grafana
  - [x] Jaeger 链路追踪

---

## M1: 现货交易闭环 ✅

### M1.1 exchange-user（用户服务）✅
- [x] 数据模型（User/UserSecurity/ApiKey）
- [x] API 实现
  - [x] `POST /v1/auth/register`
  - [x] `POST /v1/auth/login`
  - [x] `POST /v1/apiKeys`（创建）
  - [x] `GET /v1/apiKeys`（列表）
  - [x] `DELETE /v1/apiKeys/{id}`
- [x] 密码哈希（bcrypt）
- [x] API Key 生成与验证

### M1.2 exchange-gateway（API 网关）✅
- [x] REST 路由代理
- [x] 签名验证中间件（HMAC-SHA256）
- [x] 时间戳 + nonce 防重放
- [x] IP/用户限流中间件
- [x] CORS 支持

### M1.3 exchange-order（订单服务）✅
- [x] 数据模型（Order/SymbolConfig）
- [x] 下单流程（参数校验、幂等、发送到撮合）
- [x] 撤单流程
- [x] 订单查询（order/openOrders/allOrders）

### M1.4 exchange-matching（撮合引擎）✅
- [x] 内存订单簿（双向链表 + 价格档位 map）
- [x] 价格优先、时间优先撮合
- [x] LIMIT/MARKET 订单支持
- [x] 事件输出（OrderAccepted/TradeCreated/OrderCanceled）
- [x] 按 symbol 分片（独立 goroutine）

### M1.5 exchange-clearing（清算服务）✅
- [x] 数据模型（AccountBalance/LedgerEntry）
- [x] 资金冻结/解冻
- [x] 成交清算（含手续费）
- [x] 幂等键保证
- [x] 余额查询

---

## M2: 行情闭环 ✅

### M2.1 exchange-marketdata ✅
- [x] 消费撮合事件（TradeCreated/OrderAccepted）
- [x] 内存盘口维护
- [x] REST 接口
  - [x] `GET /v1/depth`
  - [x] `GET /v1/trades`
  - [x] `GET /v1/ticker`
- [x] WebSocket 推送
  - [x] 订阅/取消订阅
  - [x] `market.{symbol}.book`
  - [x] `market.{symbol}.trades`
  - [x] 心跳检测

---

## M3: 后台与止血能力 🔲

### M3.1 exchange-admin
- [ ] RBAC 权限模型
- [ ] 配置管理（交易对/费率/风控规则）
- [ ] Kill Switch（全局/按 symbol 暂停）
- [ ] 审计日志
- [ ] 提现审核
- [ ] 人工加减款（双人复核）

---

## M4: 钱包出入金 🔲

### M4.1 exchange-wallet
- [ ] 资产/网络配置
- [ ] 充值（地址生成、链上监听、去重入账）
- [ ] 提现（申请、风控、审批、出款）
- [ ] 链上对账
- [ ] 地址白名单

---

## M5: 合规与安全加强 🔲

- [ ] KYC 状态机
- [ ] AML 接口预留
- [ ] 数据留存策略
- [ ] 安全审计

---

## 启动方式

```bash
# 1. 一键启动（包含：启动 dev infra、跑迁移、编译并启动全部服务）
bash exchange-common/scripts/start-all.sh start

# 2. 单独调试某个服务（可选）
cd exchange-user && go run ./cmd/user          # :8085
```

---

## 数据流

```
Client -> Gateway(8080) -> Order(8081) -> Redis Stream(exchange:orders)
                                              |
                                              v
                                    Matching(8082)
                                              |
                                              v
                                    Redis Stream(exchange:events)
                                              |
                    +-------------------------+-------------------------+
                    |                         |                         |
                    v                         v                         v
            Clearing(8083)           MarketData(8084)            Order(状态更新)
                    |                         |
                    v                         v
              PostgreSQL              WS Push(8094)
```

---

## 验收标准（DoD）

每个功能点必须满足：
1. **文档**：接口/数据模型/事件字段写清
2. **功能**：核心路径跑通（含异常分支）
3. **幂等**：重复请求不会导致资金或状态错误
4. **审计**：关键行为有日志与审计记录
5. **可观测**：关键指标可见（延迟、队列、拒单）
6. **开关**：可随时关闭高风险功能

---

## 风险点

1. **资金一致性**：清算/账本写入并发竞态、幂等失效
2. **撮合性能**：热点 symbol 处理、内存订单簿恢复
3. **行情延迟**：事件积压、WS 推送瓶颈
4. **出入金安全**：链上监听可靠性、提现审批流程
