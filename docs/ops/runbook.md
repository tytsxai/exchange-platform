# Runbook（生产运行手册）

目标：项目在 **生产环境长期稳定运行**，出现故障时能 **快速定位、止血、回滚、恢复**。

说明：本仓库的部署目标为**后端服务**；如需 Web/管理后台前端，请作为独立项目部署并通过网关接入。

## 0. 上线前必须确认（缺一会影响结论/方案）

1. **部署形态**：生产最终是 `docker compose` / `Kubernetes` / `systemd`？是否需要多副本？
2. **网络边界**：是否保证只有 `exchange-gateway`（以及可选 marketdata WS）对公网暴露？
3. **数据目标**：Postgres/Redis 的 RPO/RTO 目标（例如 RPO<=5min, RTO<=30min）？
4. **TLS 与合规**：是否要求 DB/Redis 也必须 TLS？是否需要审计留存与访问控制？

## 1. 发布前预检（必做）

- 准备生产环境变量：复制 `deploy/prod/prod.env.example` → `deploy/prod/prod.env`，填入真实值
- 初始化/迁移数据库（必须做一次，避免缺表/缺 schema 导致服务启动即异常）：
  - 有 `psql`：设置 `DB_URL=postgres://...` 后运行 `bash exchange-common/scripts/migrate.sh`
  - 没有 `psql`：用临时容器执行（示例）：
    - `docker run --rm -v "$(pwd)/exchange-common:/work" -w /work postgres:16-alpine sh -lc "apk add --no-cache bash >/dev/null && DB_URL=postgres://... bash scripts/migrate.sh"`
- 运行预检脚本（防止误用 dev 默认值）：
  - `bash exchange-common/scripts/prod-preflight.sh`
  - 或指定 env 文件：`PROD_ENV_FILE=deploy/prod/prod.env bash deploy/prod/deploy.sh`（脚本会读取该文件做 preflight）

## 2. 部署（Docker Compose）

建议：生产优先使用 **外部/托管 Postgres + 托管 Redis**（TLS + 密码），应用只部署服务容器。

- 启动/更新：
  - 推荐一键脚本：`bash deploy/prod/deploy.sh`（会先 preflight；如设置了 `DB_URL` 也会先 migrate）
  - 或手工：`docker compose -f deploy/prod/docker-compose.yml --env-file deploy/prod/prod.env up -d --build`
- 查看状态（含 healthcheck）：
  - `docker compose -f deploy/prod/docker-compose.yml ps`
- 查看日志：
  - `docker compose -f deploy/prod/docker-compose.yml logs -f gateway`

## 2.1 监控（可选，但生产强烈建议）

- 启动 Prometheus + Grafana（默认不暴露端口，仅内网可达）：
  - `cp deploy/prod/monitoring.env.example deploy/prod/monitoring.env`
  - 填写 `GRAFANA_ADMIN_PASSWORD`
  - `docker compose -f deploy/prod/docker-compose.monitoring.yml --env-file deploy/prod/monitoring.env up -d`
  - 注意：该监控 compose 依赖 `exchange-prod-net`；先启动应用 compose（或手动 `docker network create exchange-prod-net`）

如果启用了 `METRICS_TOKEN`：
- 将 token 写入 `deploy/prod/metrics.token`（已被 gitignore）
- 在 `deploy/prod/prometheus.yml` 里取消注释 `bearer_token_file`

## 3. 上线后验收（最小可交付）

- 网关可用：
  - `curl -sf http://<gateway-host>:8080/health`
  - `curl -sf http://<gateway-host>:8080/ready`
- 内部关键服务 ready 绿（至少覆盖异步链路）：
  - `matching/order/clearing/marketdata` 的 `/ready` 会包含 **Streams 消费循环活性**，如果消费 goroutine 卡死/退出会返回 `503`
- 网关私有推送链路：
  - `gateway` 的 `/ready` 会包含 `privateEventsConsumer`（Redis Pub/Sub 消费循环活性）；down 时私有 WS 推送可能中断
  - 私有 WS 路径：`/ws/private`（默认端口：8090）
- 关键链路 E2E（建议复用项目自带脚本/用例）：
  - 注册/登录 → 下单 → 撮合 → 清算 → 查询资产/订单
- 指标可抓取（建议只在内网/反代后访问）：
  - `curl -sf http://<gateway-host>:8080/metrics`
  - Prometheus 抓取示例配置：`deploy/prod/prometheus.yml`（告警规则：`deploy/prod/alerts.yml`）

## 4. 回滚（最小可执行）

原则：**先回滚应用，再处理数据**。数据库变更必须向前兼容（先加后删，删字段延后）。

- 回滚到上一版本镜像/代码（compose）：
  1. 将 `deploy/prod/docker-compose.yml` 的 build 版本切回上一 tag（或用上一份构建产物）
  2. `docker compose -f deploy/prod/docker-compose.yml --env-file deploy/prod/prod.env up -d --build`
- 回滚后检查：
  - `/ready` 恢复、关键链路通过、错误率回落

如果你使用 `APP_VERSION`（推荐）：
- 将 `deploy/prod/prod.env` 里的 `APP_VERSION` 改回上一个 tag
- 重新执行：`bash deploy/prod/deploy.sh`（或 `docker compose ... up -d --build`）

## 5. 常见故障排查入口（建议优先）

- **服务是否还活着**：`docker compose ps`
- **健康检查是否失败**：`docker compose ps` 的 `healthy/unhealthy`
- **依赖是否异常**：各服务 `/ready` 返回 `degraded` 时查看 dependencies
- **Redis Streams backlog**：
  - 查看 pending、DLQ（stream: `exchange:*:dlq`）
  - 优先确认：consumer group/name 是否配置错误（同名 consumer 会互相抢占）
- **消费者活性 down（/ready 里出现 eventStreamConsumer/orderStreamConsumer down）**：
  - 看对应服务日志中是否有 `panic` / `read stream error`
  - 校验 `*_CONSUMER_GROUP` / `*_CONSUMER_NAME` 是否按副本唯一
  - 检查 Redis 是否慢/断连导致持续失败；必要时先扩容 Redis/降低负载再重启消费者

## 6. 安全操作要点（最低基线）

- 只允许 `exchange-gateway`（以及可选 marketdata WS）对公网暴露，其余服务仅内网可达
- `ENABLE_DOCS=false`（除非明确需要并且有额外保护）
- `/metrics` 建议只走内网抓取；如必须暴露，请配置 `METRICS_TOKEN`
- 真实客户端 IP：网关仅在“可信上游代理”（loopback/私网）场景信任 `X-Forwarded-For`；生产需确保反代会覆盖/清理客户端传入的 XFF
- 如暴露 `marketdata` public WS（8094），务必设置 `MARKETDATA_WS_ALLOW_ORIGINS`（禁止 `*`），并避免把它直接裸奔在公网
- 定期轮换 `INTERNAL_TOKEN` / `AUTH_TOKEN_SECRET` / `ADMIN_TOKEN` 并验证回滚路径
