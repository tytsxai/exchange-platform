# 生产就绪（Ready）清单与补强建议

面向目标：**马上要上线到生产环境并长期稳定运行**，在不破坏现有设计前提下，把“必炸点”先补齐到可交付状态。

## 0. 先确认的关键问题（缺一会影响结论）

1. **部署形态**：生产是 `Kubernetes` / `docker-compose` / `systemd` / 物理机直跑？是否支持多副本？
2. **网络边界**：是否保证只有 `exchange-gateway` 对公网暴露，其他服务只在内网可访问？
3. **数据库与 Redis**：是否有主从/高可用？备份频率、保留周期、RPO/RTO 目标是什么？
4. **安全合规要求**：是否必须全链路 TLS（含 Redis/Postgres）？是否需要审计留存与访问控制？

> 下面的建议默认：**网关对外，其余服务不对公网**；DB/Redis 有基本的 HA 或至少可恢复；需要可观测、可回滚、可运维。

## 1. P0（现在不修，上线/运行中一定会出问题）

### 1.1 默认密钥/弱配置防呆

- 非 `dev` 环境禁止使用默认占位符密钥（`INTERNAL_TOKEN`、`AUTH_TOKEN_SECRET`、`ADMIN_TOKEN`）。
- 非 `dev` 环境强制 `DB_SSL_MODE != disable`、`DB_PASSWORD != exchange123`。
- 非 `dev` 环境强制设置 `REDIS_PASSWORD`（避免“内网裸奔”配置被错误带到公网/跨机环境）。

落地方式：
- 代码侧 `Config.Validate()` fast-fail（避免带病启动）。
- 上线前运行 `exchange-common/scripts/prod-preflight.sh`（避免误配进入发布流程）。

### 1.2 文档与指标的暴露面

- 默认关闭 `/docs` 和 `/openapi.yaml`（非 dev 环境），只有在明确允许时才开启。
- `/metrics` 建议走内网抓取；如必须暴露，至少用 `METRICS_TOKEN` 做最小鉴权。

### 1.3 内部 HTTP 调用“必须有超时 + 鉴权一致”

- 所有服务间 HTTP 调用必须设置超时。
- 调用匹配/清算等内部接口必须携带 `X-Internal-Token`，否则某些路径在生产上会“默默失效”。

## 2. P1（不修会导致长期不稳定/难运维）

### 2.1 Redis Streams 消费稳定性

- 消费组重启后 pending 消息处理策略（claim/重试/DLQ）需要明确并可观测。
- 明确每个 consumer 的命名规则与部署副本数（防止同名 consumer 互相踢）。

### 2.2 数据一致性与恢复演练

- 备份：Postgres 定时备份 + 恢复演练（至少月度）。
- 对账：清算侧 reconciliation CLI 定期跑并告警（已有 cron 示例，可补齐运行说明）。

### 2.3 回滚与变更控制

- 数据库变更：只允许向前兼容（先加字段/表，再灰度代码；删字段要延后）。
- 回滚：保留上一版本二进制/镜像 + 可回滚配置；若迁移不可逆，必须提供“降级路径”说明。

## 3. P2（质量门槛与文档）

- CI：`go test ./...` + `go vet ./...` + `gofmt -w` 作为合并门槛。
- Runbook：部署、扩容、故障排查、备份恢复、告警处理步骤。
- 安全基线：最小权限原则、密钥轮换流程、日志脱敏要求。

## 4. 上线前建议的最小流程（可执行）

1. 准备生产环境变量（不要复用 dev `.env`）。
2. 运行预检：
   - `bash exchange-common/scripts/prod-preflight.sh`
3. 部署基础设施（Postgres/Redis/Prometheus/Grafana/Jaeger）。
4. 部署服务（先内部服务，再网关），并检查：
   - `/ready` 全绿
   - 关键链路 E2E（下单/撮合/清算/查询）
5. 开启告警（实例存活、stream pending/DLQ、handler errors）。
6. 做一次备份 + 恢复演练（至少在 staging）。

