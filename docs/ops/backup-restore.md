# 备份与恢复（Postgres / Redis）

说明：本项目的“真数据”在 Postgres；Redis 主要承载 Streams/PubSub/Nonce 等运行态数据。生产建议使用 **托管 Postgres + 托管 Redis** 并启用快照/PITR/多副本。

## 1. 必须先明确的目标（否则无法验收）

- **RPO**：最多允许丢失多少数据（必须写成明确数值，例如 `RPO <= 5 分钟`）
- **RTO**：最多允许中断多久（必须写成明确数值，例如 `RTO <= 30 分钟`）
- **保留周期**：备份保留多少天/周/月
- **演练频率**：建议至少每月一次在 staging 做恢复演练

上线前至少完成一次“全流程恢复演练”，并形成书面记录。可直接使用模板：
- `docs/ops/backup-restore-drill-template.md`

## 2. Postgres（强制）

### 2.1 备份建议（推荐顺序）

1. **托管能力优先**：开启自动备份 + PITR（Point-In-Time Recovery）+ 多可用区（如有）
2. **补充逻辑备份**：定期 `pg_dump`（用于跨环境恢复、应急导出）

### 2.2 手工备份（pg_dump 示例）

生产建议：
- 显式使用完整 `DB_URL`（包含 `sslmode=require`），避免依赖本地默认 host/port 误操作。
- 项目脚本 `exchange-common/scripts/backup-db.sh` 在 `APP_ENV!=dev` 且未提供 `DB_URL` 时会拒绝执行。
- 项目脚本支持 `KEEP_DAYS`（默认 30）自动清理历史备份，避免备份目录长期膨胀导致磁盘风险。
- 备份脚本会校验输出文件非空，空文件会直接失败（避免“看起来执行成功但实际无备份”）。

- 逻辑备份（自包含，便于恢复）：
  - `pg_dump -Fc -h <host> -p 5432 -U <user> -d exchange -f exchange.dump`
- 仅导出 schema（用于对比/审计）：
  - `pg_dump -s -h <host> -p 5432 -U <user> -d exchange -f exchange.schema.sql`

### 2.3 恢复（pg_restore 示例）

生产建议：
- 恢复时使用目标环境的显式 `DB_URL`；不要依赖脚本内置默认本地连接串。
- 项目脚本 `exchange-common/scripts/restore-db.sh` 在 `APP_ENV!=dev` 且未提供 `DB_URL` 时会拒绝执行。

1. 新建空库（或新实例）：
   - `createdb -h <host> -p 5432 -U <user> exchange`
2. 恢复：
   - `pg_restore -h <host> -p 5432 -U <user> -d exchange --clean --if-exists exchange.dump`
3. 验收：
   - 服务 `/ready` 全绿
   - 关键链路 E2E 通过
   - 对账/清算工具运行无异常（如有）

## 3. Redis（强烈建议）

### 3.1 风险点

- Redis Streams backlog（pending/DLQ）会影响撮合、订单更新、行情等异步链路
- Nonce/限流等运行态数据丢失通常可接受，但 Streams 丢失会导致业务状态不一致

### 3.2 托管 Redis 建议

- 开启持久化（AOF/RDB）或使用具备自动快照/复制的托管服务
- 保障认证与 TLS（跨机/跨网段场景）
- 明确升级/故障切换对 Streams 的影响窗口
- 若使用 `exchange-common/scripts/backup-redis.sh`，同样可通过 `KEEP_DAYS`（默认 30）自动清理旧快照

### 3.3 恢复后验收要点

- 所有 consumer group 的 `/ready` 正常
- 检查 DLQ 是否突然增长（`*:dlq`）
- 检查 pending 是否持续增加（消费停滞）

## 4. 必须纳入备份的“配置与密钥”

- `deploy/prod/prod.env`（或等效密钥管理系统中的条目）
- 反代/TLS 配置（Nginx/Caddy/LB）
- 发布配置（镜像 tag、compose/k8s 配置）

## 5. 对账/一致性演练（强烈建议）

项目已内置清算对账工具，可用于生产定期校验：

```bash
go run exchange-clearing/cmd/reconciliation \
  --db-url "postgres://user:pass@host:5432/exchange?sslmode=require" \
  --alert=true \
  --report /tmp/reconciliation-report.json
```

建议：
- 生产最少每日/每小时运行一次（按业务量调整）
- 发现差异立刻告警（P0），先止血再修复
- 修复原则见运行手册与资金账本 ADR

## 6. 演练记录与审计（上线阻断）

- 每次演练必须记录：
  - 演练时间、执行人、环境
  - 目标 RPO/RTO 与实际 RPO/RTO
  - 失败点与修复动作
  - 关键日志与对账报告路径
- 建议将演练记录存放在 `docs/ops/drills/` 并按日期命名，方便审计与复盘。
