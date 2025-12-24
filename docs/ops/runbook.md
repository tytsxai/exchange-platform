# Runbook（生产运行手册）

目标：项目在 **生产环境长期稳定运行**，出现故障时能 **快速定位、止血、回滚、恢复**。

## 0. 上线前必须确认（缺一会影响结论/方案）

1. **部署形态**：生产最终是 `docker compose` / `Kubernetes` / `systemd`？是否需要多副本？
2. **网络边界**：是否保证只有 `exchange-gateway`（以及可选 marketdata WS）对公网暴露？
3. **数据目标**：Postgres/Redis 的 RPO/RTO 目标（例如 RPO<=5min, RTO<=30min）？
4. **TLS 与合规**：是否要求 DB/Redis 也必须 TLS？是否需要审计留存与访问控制？

## 1. 发布前预检（必做）

- 准备生产环境变量：复制 `deploy/prod/prod.env.example` → `deploy/prod/prod.env`，填入真实值
- 运行预检脚本（防止误用 dev 默认值）：
  - `bash exchange-common/scripts/prod-preflight.sh`

## 2. 部署（Docker Compose）

建议：生产优先使用 **外部/托管 Postgres + 托管 Redis**（TLS + 密码），应用只部署服务容器。

- 启动/更新：
  - `docker compose -f deploy/prod/docker-compose.yml --env-file deploy/prod/prod.env up -d --build`
- 查看状态（含 healthcheck）：
  - `docker compose -f deploy/prod/docker-compose.yml ps`
- 查看日志：
  - `docker compose -f deploy/prod/docker-compose.yml logs -f gateway`

## 3. 上线后验收（最小可交付）

- 网关可用：
  - `curl -sf http://<gateway-host>:8080/health`
  - `curl -sf http://<gateway-host>:8080/ready`
- 关键链路 E2E（建议复用项目自带脚本/用例）：
  - 注册/登录 → 下单 → 撮合 → 清算 → 查询资产/订单
- 指标可抓取（建议只在内网/反代后访问）：
  - `curl -sf http://<gateway-host>:8080/metrics`

## 4. 回滚（最小可执行）

原则：**先回滚应用，再处理数据**。数据库变更必须向前兼容（先加后删，删字段延后）。

- 回滚到上一版本镜像/代码（compose）：
  1. 将 `deploy/prod/docker-compose.yml` 的 build 版本切回上一 tag（或用上一份构建产物）
  2. `docker compose -f deploy/prod/docker-compose.yml --env-file deploy/prod/prod.env up -d --build`
- 回滚后检查：
  - `/ready` 恢复、关键链路通过、错误率回落

## 5. 常见故障排查入口（建议优先）

- **服务是否还活着**：`docker compose ps`
- **健康检查是否失败**：`docker compose ps` 的 `healthy/unhealthy`
- **依赖是否异常**：各服务 `/ready` 返回 `degraded` 时查看 dependencies
- **Redis Streams backlog**：
  - 查看 pending、DLQ（stream: `exchange:*:dlq`）
  - 优先确认：consumer group/name 是否配置错误（同名 consumer 会互相抢占）

## 6. 安全操作要点（最低基线）

- 只允许 `exchange-gateway`（以及可选 marketdata WS）对公网暴露，其余服务仅内网可达
- `ENABLE_DOCS=false`（除非明确需要并且有额外保护）
- `/metrics` 建议只走内网抓取；如必须暴露，请配置 `METRICS_TOKEN`
- 定期轮换 `INTERNAL_TOKEN` / `AUTH_TOKEN_SECRET` / `ADMIN_TOKEN` 并验证回滚路径

