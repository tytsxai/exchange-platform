# 备份与恢复（Postgres / Redis）

说明：本项目的“真数据”在 Postgres；Redis 主要承载 Streams/PubSub/Nonce 等运行态数据。生产建议使用 **托管 Postgres + 托管 Redis** 并启用快照/PITR/多副本。

## 1. 必须先明确的目标（否则无法验收）

- **RPO**：最多允许丢失多少数据（例如 5 分钟）
- **RTO**：最多允许中断多久（例如 30 分钟）
- **保留周期**：备份保留多少天/周/月
- **演练频率**：建议至少每月一次在 staging 做恢复演练

## 2. Postgres（强制）

### 2.1 备份建议（推荐顺序）

1. **托管能力优先**：开启自动备份 + PITR（Point-In-Time Recovery）+ 多可用区（如有）
2. **补充逻辑备份**：定期 `pg_dump`（用于跨环境恢复、应急导出）

### 2.2 手工备份（pg_dump 示例）

- 逻辑备份（自包含，便于恢复）：
  - `pg_dump -Fc -h <host> -p 5432 -U <user> -d exchange -f exchange.dump`
- 仅导出 schema（用于对比/审计）：
  - `pg_dump -s -h <host> -p 5432 -U <user> -d exchange -f exchange.schema.sql`

### 2.3 恢复（pg_restore 示例）

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
