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
- 非 `dev` 环境关键密钥建议统一最小长度（默认 `32`）：`INTERNAL_TOKEN`、`AUTH_TOKEN_SECRET`、`ADMIN_TOKEN`、`API_KEY_SECRET_KEY`。
- 非 `dev` 环境默认禁止使用 `APP_VERSION=latest` 直接发布（避免版本不可追踪、回滚不可控）。
- 非 `dev` 环境强制 `DB_SSL_MODE != disable`、`DB_PASSWORD != exchange123`。
- 非 `dev` 环境强制设置 `REDIS_PASSWORD`（避免“内网裸奔”配置被错误带到公网/跨机环境）。
- 若生产 Redis 为 TLS-only（托管 Redis 常见），需开启 `REDIS_TLS=true`；服务已支持 `REDIS_CACERT/REDIS_CERT/REDIS_KEY/REDIS_SERVER_NAME`。
  - 预检会校验 `REDIS_TLS` 为合法布尔值，且 `REDIS_CERT/REDIS_KEY` 必须成对配置。
 - API Key secret 以对称加密方式存储；切换加密密钥需评估已有 Key 的迁移或重置流程。

落地方式：
- 代码侧 `Config.Validate()` fast-fail（避免带病启动）。
- 上线前运行 `exchange-common/scripts/prod-preflight.sh`（避免误配进入发布流程，可通过 `MIN_SECRET_LENGTH` 统一最小长度策略；默认要求 `APP_VERSION` 不是 `latest`）。

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
- `gateway` 私有事件消费者（用于 `/ws/private` 推送）异常退出后会自动重连，避免“推送链路静默中断”长期悬挂。
- 注意：`docker compose` 不会因为 `unhealthy` 自动重启容器；若生产仍使用 compose，需配合外部守护或告警自动化执行重启。

### 1.5 数据库 schema 初始化/迁移必须纳入发布流程

本项目服务启动不会自动建表；如果生产库未初始化，服务会在运行期报错（缺 schema/缺表），并且可能产生“部分服务可用、部分链路不可用”的灰色状态。

落地方式（必须执行一次）：
- 设置 `DB_URL=postgres://...` 后运行：`bash exchange-common/scripts/migrate.sh`
- 或使用临时容器执行（无 `psql` 环境）：见 `docs/ops/runbook.md`
- 若发布时确认“本次不跑迁移”，需显式设置：`RUN_MIGRATIONS=false MIGRATIONS_SKIP_ACK=true`（避免误跳过迁移）

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

### 1.9 API Key 权限必须在网关强制执行（防越权）

风险：如果网关只做签名校验、不校验权限位（READ/TRADE/WITHDRAW），
则“只读 Key”也可能发起下单/撤单等交易写操作，属于典型越权。

落地方式（已实现）：
- 网关私有路由按 method 强制权限：
  - `/v1/order`: `GET=READ`，`POST/DELETE=TRADE`
  - `/v1/openOrders` `/v1/allOrders` `/v1/myTrades` `/v1/account` `/v1/ledger`: `READ`
- 私有 WebSocket `/ws/private` 连接要求至少具备 `READ` 权限。

### 1.10 用户级限流必须在鉴权之后执行（防限流“退化成按 IP”）

风险：若 user limiter 在 Auth 之前执行，拿不到 `userID`，会退化为按 IP 限流，
导致同一用户跨 IP 绕限、或多用户共享出口 IP 时互相影响。

落地方式（已实现）：
- 私有路由中间件顺序调整为：`Auth -> UserRateLimit -> Handler`。

### 1.11 清算侧禁止硬编码交易对资产与精度（防错账）

风险：清算若硬编码 `USDT` 和固定 `1e8` 精度，遇到非 USDT 或不同精度交易对会算错 `quoteQty`，
可能造成冻结/结算金额错误，属于资金一致性事故。

落地方式（已实现）：
- 成交流处理时改为从 `exchange_order.symbol_configs` 解析 `base_asset / quote_asset / qty_precision`。
- 使用精度驱动的 `quoteQty` 计算（含溢出保护），替换硬编码逻辑。

### 1.12 账户余额首写并发竞态需重试（防随机失败）

风险：首次写入 `(user_id, asset)` 时并发请求可能同时 INSERT，
其中一个会触发唯一键冲突并直接失败，导致“偶发下单/入账失败”。

落地方式（已实现）：
- 在余额仓储中将首写唯一键冲突识别为 `ErrOptimisticLockFailed`，交由上层重试流程处理。

### 1.13 Admin API 必须在入口强制 RBAC（防高危操作被越权调用）

风险：如果 admin 仅依赖 `Bearer + X-Admin-Token`，缺少每个接口的权限校验，
则持有通用 admin token 的用户可能执行不属于自己职责的高危操作（如 kill switch、交易对配置修改）。

落地方式（已实现）：
- `exchange-admin` 新增入口 RBAC 中间件，对 `/admin/*` 按 `method + route` 强制权限检查。
- 权限来自 `exchange_admin.user_roles -> roles.permissions` 聚合结果，支持 `*` 超级权限。
- 采用默认拒绝（fail-closed）：新增 admin 路由若未配置权限规则，将直接拒绝，避免“忘记加鉴权即裸露”。

### 1.14 告警必须闭环到值班人（防“只告警不通知”）

风险：仅有 Prometheus 规则但未接通知通道时，异步链路故障可能长期无人感知。

落地方式（已实现模板）：
- 监控栈已包含 `Alertmanager`：`deploy/prod/docker-compose.monitoring.yml`
- Prometheus 已接入告警路由：`deploy/prod/prometheus.yml` 的 `alerting` 段
- 告警路由配置：`deploy/prod/alertmanager.yml`（需替换为真实 on-call 通道）
- 发布门禁默认会执行：`scripts/check-alertmanager-config.sh`（阻止占位符 webhook 地址误上线）
- 人工触发演练脚本：`deploy/prod/alert-drill.sh fire|resolve`

### 1.15 镜像发布/回滚必须校验“目标 tag 可拉取”（防回滚时镜像缺失）

风险：`deploy.sh/rollback.sh` 可执行但目标镜像不可拉取，会导致发布/回滚在事故时失效。

落地方式（已实现）：
- 新增镜像可拉取校验：`deploy/prod/check-images.sh`
- `deploy.sh` / `rollback.sh` 在非 dev 的 image-only 流程会默认执行可拉取检查（`VERIFY_IMAGE_PULL=true`）
- 支持镜像仓库前缀：`IMAGE_REPOSITORY_PREFIX`（例如 `ghcr.io/your-org/`）
- 镜像构建推送 workflow 示例：`.github/workflows/release-images.yml`

### 1.16 Compose 自愈与持续 unhealthy 告警必须并行落地

风险：`docker compose` 不会自动重启 `unhealthy`，消费者卡死可能长期挂起。

落地方式（已实现模板）：
- 自愈脚本：`deploy/prod/restart-unhealthy.sh`
- 定时化模板：
  - `cron` 示例：`exchange-common/scripts/cron.example`
  - `systemd timer` 示例：`deploy/prod/systemd/exchange-restart-unhealthy.{service,timer}`
- 持续 unhealthy 告警：
  - 已引入 `blackbox-exporter` 探测 `/ready`
  - 新增 `ServiceReadyProbeFailed` 告警规则（持续 3 分钟失败触发）

### 1.17 网络边界与 TLS 必须纳入发布门禁

风险：内部服务误暴露公网会扩大攻击面；TLS 未验收会引入中间人风险与合规风险。

落地方式（已实现模板）：
- 公网暴露端口门禁：`scripts/check-public-exposure.sh`（默认仅允许 gateway，marketdata 需显式允许）
- TLS 验收可通过：`scripts/prod-verify.sh` 的 `PUBLIC_API_HTTPS_URL` / TLS 握手选项
- 如暴露 marketdata WS，边缘限流示例：`deploy/prod/nginx.marketdata-ws.conf.example`
- 发布门禁 workflow 示例：`.github/workflows/prod-release-gate.yml`

### 1.18 备份恢复目标必须量化并留演练记录

风险：未定义 RPO/RTO 与演练记录时，事故恢复无法验收。

落地方式（已实现模板）：
- `docs/ops/backup-restore.md` 已要求明确数值化 RPO/RTO
- 演练记录模板：`docs/ops/backup-restore-drill-template.md`

## 2. P1（不修会导致长期不稳定/难运维）

### 2.1 Redis Streams 消费稳定性

- 消费组重启后 pending 消息处理策略（claim/重试/DLQ）需要明确并可观测。
- 明确每个 consumer 的命名规则与部署副本数（防止同名 consumer 互相踢）。
- 订单去重 TTL：`MATCHING_ORDER_DEDUP_TTL`（默认 24h），用于防止 Streams 重试导致重复下单。
- 去重键状态采用 `processing -> done`，避免实例异常退出后“重复消息被误 ACK 丢弃”。

### 2.1.1 撮合启动恢复（已补齐）

- `exchange-matching` 已接入 `OrderLoader`，可在启动时从 `exchange_order.orders`
  恢复 `NEW/PARTIALLY_FILLED` 的 LIMIT 挂单，降低“ACK 后进程异常”导致的挂单丢失风险。
- 生产建议开启：`MATCHING_RECOVERY_ENABLED=true`（默认生产示例已开启）。

### 2.2 数据一致性与恢复演练

- 备份：Postgres 定时备份 + 恢复演练（至少月度）。
- 备份保留：`backup-db.sh / backup-redis.sh` 默认支持 `KEEP_DAYS=30` 自动清理历史备份，避免磁盘被备份打满。
- 对账：清算侧 reconciliation CLI 定期跑并告警（已有 cron 示例，可补齐运行说明）。

### 2.3 回滚与变更控制

- 数据库变更：只允许向前兼容（先加字段/表，再灰度代码；删字段要延后）。
- 回滚：保留上一版本二进制/镜像 + 可回滚配置；若迁移不可逆，必须提供“降级路径”说明。
- 生产发布默认应走 **image-only**（不源码构建），确保 `APP_VERSION` 具备可追踪与可回滚语义。

### 2.4 Liveness 与 Readiness 分离（避免“依赖抖动”触发误重启）

- `/live`：仅表示进程存活，不依赖 DB/Redis/下游（用于容器 liveness/进程探活）。
- `/ready`：包含依赖与消费循环健康（用于流量接入/编排就绪）。
- 若使用编排系统，建议将 **liveness 指向 `/live`**，**readiness 指向 `/ready`**，避免下游短暂波动导致自身被重启。

### 2.5 提现状态更新需使用条件更新（防并发覆盖状态）

风险：如果提现状态更新仅按 `withdraw_id` 覆盖，审批/拒绝/完成在并发下会出现“后写覆盖先写”，
进而造成资金状态与业务状态不一致（例如并发审批与拒绝）。

落地方式（已实现）：
- `wallet` 仓储新增 `UpdateWithdrawalStatusCAS`，SQL 条件包含 `status = ANY(expected_statuses)`。
- 服务层统一走条件更新，若命中失败会回读最新状态：
  - 已是目标状态：按幂等成功处理；
  - 非目标状态：返回 `INVALID_WITHDRAW_STATE`，避免静默覆盖。

### 2.6 容器日志必须滚动（防宿主机磁盘耗尽）

风险：Docker 默认 `json-file` 日志不限制大小，长期运行后高概率打满宿主机磁盘，导致容器异常、节点不可用。

落地方式（已实现）：
- `deploy/prod/docker-compose.yml` 与 `deploy/prod/docker-compose.monitoring.yml` 已启用日志滚动：
  - `DOCKER_LOG_MAX_SIZE`（默认 `20m`）
  - `DOCKER_LOG_MAX_FILE`（默认 `5`）

## 3. P2（质量门槛与文档）

- CI：`go test ./...` + `go vet ./...` + `gofmt -w` 作为合并门槛。
- Runbook：部署、扩容、故障排查、备份恢复、告警处理步骤。
- 安全基线：最小权限原则、密钥轮换流程、日志脱敏要求。
- 版本与回滚：生产建议使用 `APP_VERSION` 镜像 tag（见 `deploy/prod/prod.env.example`），确保回滚是“改 tag → 重启”。

## 4. 上线前建议的最小流程（可执行）

1. 准备生产环境变量（不要复用 dev `.env`）。
2. 运行预检：
   - `bash exchange-common/scripts/prod-preflight.sh`
3. 运行发布门禁：
   - `PROD_ENV_FILE=deploy/prod/prod.env bash scripts/prod-release-gate.sh`
   - 首次上线前若环境尚未部署：`RUN_PROD_VERIFY=0 PROD_ENV_FILE=deploy/prod/prod.env bash scripts/prod-release-gate.sh`
   - 若使用外部监控系统且不依赖仓库内 Alertmanager 配置：`RUN_ALERTMANAGER_CHECK=0`
4. 部署基础设施（Postgres/Redis/Prometheus/Alertmanager/Grafana/Jaeger）。
   - 首次部署后执行一次：`bash deploy/prod/alert-drill.sh fire` 做通知链路演练
5. 部署服务（先内部服务，再网关），并检查：
   - `/ready` 全绿
   - 关键链路 E2E（下单/撮合/清算/查询）
   - 可选：`bash scripts/prod-verify.sh` 做一键就绪检查
6. 开启告警（实例存活、stream pending/DLQ、handler errors、ready probe）。
7. 做一次备份 + 恢复演练（至少在 staging），并按模板留存记录。

## 5. 相关文档（建议一起看）

- 运行手册：`docs/ops/runbook.md`
- 备份与恢复：`docs/ops/backup-restore.md`
