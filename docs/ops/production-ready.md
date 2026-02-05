# 生产就绪（Ready）清单与补强建议

面向目标：**马上要上线到生产环境并长期稳定运行**，在不破坏现有设计前提下，把“必炸点”先补齐到可交付状态。

## 0. 先确认的关键问题（缺一会影响结论）

1. **部署形态**：生产是 `Kubernetes` / `docker-compose` / `systemd` / 物理机直跑？是否支持多副本？
2. **网络边界**：是否保证只有 `exchange-gateway` 对公网暴露，其他服务只在内网可访问？
3. **数据库与 Redis**：是否有主从/高可用？备份频率、保留周期、RPO/RTO 目标是什么？
4. **安全合规要求**：是否必须全链路 TLS（含 Redis/Postgres）？是否需要审计留存与访问控制？

> 下面的建议默认：**网关对外，其余服务不对公网**；DB/Redis 有基本的 HA 或至少可恢复；需要可观测、可回滚、可运维。

### 已实现的基础能力

| 能力 | 实现 | 说明 |
|------|------|------|
| 健康检查 | `pkg/health` | Live/Ready/Health 三端点分离 |
| 链路追踪 | `pkg/tracing` | OpenTelemetry + Jaeger |
| 审计日志 | `pkg/audit` | 异步写入、敏感参数自动脱敏 |
| 结构化日志 | `pkg/logger` | zerolog + traceID 关联 |
| 参数验证 | `pkg/validate` | Symbol/Price/Quantity/Address |

## 1. P0（现在不修，上线/运行中一定会出问题）

### 1.1 默认密钥/弱配置防呆

- 非 `dev` 环境禁止使用默认占位符密钥（`INTERNAL_TOKEN`、`AUTH_TOKEN_SECRET`、`ADMIN_TOKEN`、`API_KEY_SECRET_KEY`）。
- 非 `dev` 环境强制 `DB_SSL_MODE != disable`、`DB_PASSWORD != exchange123`。
- 非 `dev` 环境强制设置 `REDIS_PASSWORD`（避免“内网裸奔”配置被错误带到公网/跨机环境）。
 - API Key secret 以对称加密方式存储；切换加密密钥需评估已有 Key 的迁移或重置流程。

落地方式：
- 代码侧 `Config.Validate()` fast-fail（避免带病启动）。
- 上线前运行 `exchange-common/scripts/prod-preflight.sh`（避免误配进入发布流程）。

### 1.2 文档与指标的暴露面

- 默认关闭 `/docs` 和 `/openapi.yaml`（非 dev 环境），只有在明确允许时才开启。
- `/metrics` 建议走内网抓取；如必须暴露，至少用 `METRICS_TOKEN` 做最小鉴权。

### 1.3 内部 HTTP 调用“必须有超时 + 鉴权一致”

- 所有服务间 HTTP 调用必须设置超时。
- 调用匹配/清算等内部接口必须携带 `X-Internal-Token`，否则某些路径在生产上会“默默失效”。

### 1.4 Streams 消费循环的“活性”必须进入 /ready

只靠 `Redis/Pg Ping` 的 readiness 不够：消费者后台 goroutine 如果卡死/退出，服务可能仍对外返回 `200`，但异步链路已中断（订单不更新/撮合不消费/行情不推进）。

落地方式（已实现）：
- `matching/order/clearing/marketdata` 的 `/ready` 会额外检查 **Stream 消费循环是否在最近 45s 内 tick**；否则返回 `degraded`/`503`，触发容器编排的重启与告警。

### 1.5 数据库 schema 初始化/迁移必须纳入发布流程

本项目服务启动不会自动建表；如果生产库未初始化，服务会在运行期报错（缺 schema/缺表），并且可能产生“部分服务可用、部分链路不可用”的灰色状态。

落地方式（必须执行一次）：
- 设置 `DB_URL=postgres://...` 后运行：`bash exchange-common/scripts/migrate.sh`
- 或使用临时容器执行（无 `psql` 环境）：见 `docs/ops/runbook.md`

### 1.6 反代/负载均衡下的真实客户端 IP（X-Forwarded-For）必须“可信”

风险：如果服务直接暴露在公网且无可信反代，客户端可以伪造 `X-Forwarded-For`，导致：
- 基于 IP 的限流/封禁失效，甚至造成内存型 DoS（大量伪造 IP 产生大量 key）
- 审计/日志里的“客户端 IP”被污染，影响追查

落地方式（已实现）：
- 网关默认只在**上游为 loopback 或 RFC1918 私网**时才信任 `X-Forwarded-For`；否则忽略并使用 `RemoteAddr`。
- 若你的反代/LB 使用**公网 IP**，必须配置 `TRUSTED_PROXY_CIDRS`（逗号分隔）以显式信任该代理网段。
- 推荐生产网关前置反代/LB，并确保反代会覆盖/清理客户端传入的 XFF。

### 1.7 Public WebSocket 必须防滥用（Origin + 订阅上限 + 频道校验）

风险：如果 `marketdata` 的 public WS（8094）对公网暴露且不做限制，容易出现：
- 任意网站跨站建立 WS（浏览器会带 Origin），造成资源滥用
- 恶意订阅大量随机频道，导致服务内存持续增长

落地方式（已实现）：
- `marketdata` WS 默认只允许 `Origin` 在 `MARKETDATA_WS_ALLOW_ORIGINS` 白名单内（非 dev 禁止 `*`）；无 `Origin` 的非浏览器客户端不受影响
- 单连接订阅数上限：`MARKETDATA_WS_MAX_SUBSCRIPTIONS`（默认 50）
- 只允许订阅 `market.<SYMBOL>.(book|trades|ticker)`，并校验 SYMBOL（A-Z/0-9，长度<=32）

### 1.8 交易对精度必须与资产精度一致（资金一致性）

风险：订单/撮合/清算使用的精度与钱包资产精度不一致，会导致冻结/清算金额按错误的缩放倍数计算，
从而出现“资金凭空增减/冻结不足”等严重一致性问题。

落地方式（已实现）：
- 下单前校验：`price_precision == quote asset precision`、`qty_precision == base asset precision`；
  不一致直接拒单并提示配置错误。
- 默认示例数据已对齐精度（见 `exchange-common/scripts/init-db.sql`）。
- 存量数据库可执行：`scripts/align-symbol-precision.sql`（执行前务必备份）。

## 2. P1（不修会导致长期不稳定/难运维）

### 2.1 Redis Streams 消费稳定性

- 消费组重启后 pending 消息处理策略（claim/重试/DLQ）需要明确并可观测。
- 明确每个 consumer 的命名规则与部署副本数（防止同名 consumer 互相踢）。
- 订单去重 TTL：`MATCHING_ORDER_DEDUP_TTL`（默认 24h），用于防止 Streams 重试导致重复下单。

### 2.2 数据一致性与恢复演练

- 备份：Postgres 定时备份 + 恢复演练（至少月度）。
- 对账：清算侧 reconciliation CLI 定期跑并告警（已有 cron 示例，可补齐运行说明）。

### 2.3 回滚与变更控制

- 数据库变更：只允许向前兼容（先加字段/表，再灰度代码；删字段要延后）。
- 回滚：保留上一版本二进制/镜像 + 可回滚配置；若迁移不可逆，必须提供“降级路径”说明。

### 2.4 Liveness 与 Readiness 分离（避免“依赖抖动”触发误重启）

- `/live`：仅表示进程存活，不依赖 DB/Redis/下游（用于容器 liveness/进程探活）。
- `/ready`：包含依赖与消费循环健康（用于流量接入/编排就绪）。
- 若使用编排系统，建议将 **liveness 指向 `/live`**，**readiness 指向 `/ready`**，避免下游短暂波动导致自身被重启。

## 3. P2（质量门槛与文档）

- CI：`go test ./...` + `go vet ./...` + `gofmt -w` 作为合并门槛。
- Runbook：部署、扩容、故障排查、备份恢复、告警处理步骤。
- 安全基线：最小权限原则、密钥轮换流程、日志脱敏要求。
- 版本与回滚：生产建议使用 `APP_VERSION` 镜像 tag（见 `deploy/prod/prod.env.example`），确保回滚是“改 tag → 重启”。

## 4. 上线前建议的最小流程（可执行）

1. 准备生产环境变量（不要复用 dev `.env`）。
2. 运行预检：
   - `bash exchange-common/scripts/prod-preflight.sh`
3. 部署基础设施（Postgres/Redis/Prometheus/Grafana/Jaeger）。
4. 部署服务（先内部服务，再网关），并检查：
   - `/ready` 全绿
   - 关键链路 E2E（下单/撮合/清算/查询）
   - 可选：`bash scripts/prod-verify.sh` 做一键就绪检查
5. 开启告警（实例存活、stream pending/DLQ、handler errors）。
6. 做一次备份 + 恢复演练（至少在 staging）。

## 5. 相关文档（建议一起看）

- 运行手册：`docs/ops/runbook.md`
- 备份与恢复：`docs/ops/backup-restore.md`
