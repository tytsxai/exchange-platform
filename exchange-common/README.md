# exchange-common

交易所项目共享库：Proto 定义、工具包、数据库 Schema。

## 目录结构

```
exchange-common/
├── proto/                  # Proto 定义
│   ├── order.proto         # 订单相关
│   ├── trade.proto         # 成交相关
│   ├── account.proto       # 账户/账本相关
│   ├── market.proto        # 行情/交易对配置
│   ├── event.proto         # 事件封装
│   └── user.proto          # 用户/API Key
├── gen/                    # Proto 生成的 Go 代码
├── pkg/                    # 共享工具包
│   ├── decimal/            # 高精度计算
│   ├── errors/             # 统一错误码
│   ├── snowflake/          # 雪花 ID 生成
│   ├── signature/          # API 签名验证
│   └── redis/              # Redis Streams 封装
├── scripts/
│   ├── gen-proto.sh        # Proto 编译脚本
│   ├── init-db.sql         # 数据库初始化
│   └── prometheus.yml      # Prometheus 配置
└── docker-compose.yml      # 开发环境
```

## 快速开始

### 1. 启动开发环境

```bash
docker-compose up -d
```

服务端口：
- PostgreSQL: 5436 (user: exchange, password: exchange123)
- Redis: 6380
- Prometheus: 9090
- Grafana: 3000 (admin/admin123)
- Jaeger: 16686

### 2. 编译 Proto

```bash
chmod +x scripts/gen-proto.sh
./scripts/gen-proto.sh
```

### 3. 使用共享包

```go
import (
    "github.com/exchange/common/pkg/decimal"
    "github.com/exchange/common/pkg/errors"
    "github.com/exchange/common/pkg/snowflake"
    "github.com/exchange/common/pkg/signature"
    "github.com/exchange/common/pkg/redis"
)

// 初始化雪花 ID
snowflake.Init(1)
id := snowflake.MustNextID()

// 高精度计算
price := decimal.MustNew("100.50")
qty := decimal.MustNew("0.5")
total := price.Mul(qty)

// 错误处理
err := errors.New(errors.CodeInsufficientBalance, "余额不足")

// 签名验证
signer := signature.NewSigner("your-secret")
sig := signer.Sign(canonicalString)
```

## Proto 定义

### order.proto
- `Order`: 订单实体
- `Side`: BUY/SELL
- `OrderType`: LIMIT/MARKET
- `OrderStatus`: NEW/PARTIALLY_FILLED/FILLED/CANCELED/REJECTED
- `TimeInForce`: GTC/IOC/FOK/POST_ONLY

### account.proto
- `Balance`: 账户余额 (available/frozen)
- `LedgerEntry`: 账本流水（资金 truth）
- `LedgerReason`: 变动原因枚举

### event.proto
- `Event`: 统一事件封装
- `OrderEvent`: 订单事件
- `TradeEvent`: 成交事件
- `BookEvent`: 盘口事件
- `BalanceEvent`: 余额变动事件

### market.proto
- `SymbolConfig`: 交易对配置
- `Ticker`: 24h 行情
- `Depth`: 订单簿深度
- `Kline`: K 线

## 数据库 Schema

数据库按服务拆分为多个 schema：
- `exchange_user`: 用户、API Key
- `exchange_order`: 订单、成交、交易对配置
- `exchange_clearing`: 账户余额、账本
- `exchange_market`: K 线
- `exchange_wallet`: 资产、充值、提现
- `exchange_admin`: 审计日志、角色权限

## 错误码

详见 `pkg/errors/errors.go`，主要分类：
- 1xxx: 通用错误
- 2xxx: 签名与鉴权
- 3xxx: 限流
- 4xxx: 交易
- 5xxx: 资金
- 6xxx: 出入金
- 7xxx: 用户
- 9xxx: 系统

## 相关仓库

- `exchange-gateway`: API 网关
- `exchange-user`: 用户服务
- `exchange-order`: 订单服务
- `exchange-matching`: 撮合引擎
- `exchange-clearing`: 清算服务
- `exchange-marketdata`: 行情服务
- `exchange-admin`: 运营后台
- `exchange-wallet`: 钱包服务

## API 文档 (Swagger UI)

各服务提供在线 API 文档，前端人员可直接在浏览器中查看接口定义并进行测试。

### 访问地址

| 服务 | 端口 | 文档地址 | 说明 |
|------|------|----------|------|
| Gateway | 8080 | http://localhost:8080/docs | 主 API 入口，包含交易、行情、账户接口 |
| Admin | 8087 | http://localhost:8087/docs | 管理后台接口，交易对管理、风控、审计 |
| Wallet | 8086 | http://localhost:8086/docs | 钱包接口，充值、提现、资产管理 |

### 功能特性

- **Try it out**: 所有接口支持在线测试
- **持久化授权**: 输入 Token 后自动保存
- **搜索过滤**: 支持按关键字筛选接口
- **深度链接**: 可分享特定接口的 URL

### 认证方式

**公共接口**: 无需认证

**私有接口 (Gateway)**: HMAC-SHA256 签名
```
Headers:
  X-API-KEY: your-api-key
  X-API-TIMESTAMP: 1703232000000
  X-API-NONCE: uuid-string
  X-API-SIGNATURE: hmac-sha256-signature

Signature = HMAC-SHA256(secret, timestamp + "\n" + nonce + "\n" + METHOD + "\n" + path + "\n" + canonicalQuery)
canonicalQuery = sorted query string (exclude `signature` if present); request body is not signed.
```

**管理接口 (Admin/Wallet)**: Bearer Token
```
Headers:
  Authorization: Bearer token_{userId}
```

### OpenAPI 规范文件

- Gateway: `exchange-gateway/api/openapi.yaml`
- Admin: `exchange-admin/api/openapi.yaml`
- Wallet: `exchange-wallet/api/openapi.yaml`
