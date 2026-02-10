# 默认生产部署方案（平衡版）

你说“先别纠结形态，让我默认选一个最佳的”，这里给出一个**能尽快落地、易运维、后续可迁移**的默认方案：

## 选择

- **应用**：Docker Compose（单机/少量机器）部署 8 个 Go 服务
- **基础设施**：生产默认使用**托管 Postgres + 托管 Redis**（TLS + 密码）
- **监控（推荐）**：Prometheus + Grafana（同机或内网），配置参考 `deploy/prod/prometheus.yml`，可选 compose `deploy/prod/docker-compose.monitoring.yml`
- **对外暴露**：
  - `exchange-gateway`（HTTP 8080 + WS 8090）
  - （可选）`exchange-marketdata` 的 public WS 8094：需要你明确是否要对外；默认建议走内网或经反代限流
- **不对外暴露**：`order/matching/clearing/user/admin/wallet` 的 HTTP 端口（仅内网可达）

## 为什么这样选（“平衡”）

- 不引入 Kubernetes 的复杂度，但具备容器化发布与快速回滚能力（换镜像/版本即可）
- 将最难运维的组件（DB/Redis）交给托管服务，降低数据丢失与维护成本
- 通过网络边界 + 内部 token，把“误暴露”风险降到可控

## 你需要做的最小动作

0. 初始化/迁移数据库（必须做一次，避免服务启动后因为缺表/缺 schema “带病运行”）：
   - 有 `psql`：设置 `DB_URL=postgres://...` 后运行 `bash exchange-common/scripts/migrate.sh`
   - 没有 `psql`：用临时容器执行（示例）：
     - `docker run --rm -v "$(pwd)/exchange-common:/work" -w /work postgres:16-alpine sh -lc "apk add --no-cache bash >/dev/null && DB_URL=postgres://... bash scripts/migrate.sh"`
1. 准备生产环境变量：
   - 复制 `deploy/prod/prod.env.example` → `deploy/prod/prod.env`，填入真实值
   - 特别注意：`API_KEY_SECRET_KEY` 必填且长度 >= 32（用于 API Key secret 加密）
   - 建议设置 `APP_VERSION=<tag>`（用于镜像打标与快速回滚）
2. 运行预检（会 fast-fail 防止误配）：
   - `bash exchange-common/scripts/prod-preflight.sh`
3. 部署：
   - 推荐一键脚本：`bash deploy/prod/deploy.sh`（默认 image-only，不会 `--build`；会先 preflight）
   - 如需源码构建（建议仅 dev/应急）：`BUILD_IMAGES=true ALLOW_SOURCE_BUILD_IN_NONDEV=true bash deploy/prod/deploy.sh`
   - 无侵入演练（不真正发布）：`DRY_RUN=true bash deploy/prod/deploy.sh`
   - 如需指定 env 文件：`PROD_ENV_FILE=deploy/prod/prod.env bash deploy/prod/deploy.sh`
   - 或手工：`docker compose -f deploy/prod/docker-compose.yml --env-file deploy/prod/prod.env up -d`
4. 验证：
   - `curl -sf http://<gateway-host>:8080/live`
   - `curl -sf http://<gateway-host>:8080/health`
   - `curl -sf http://<gateway-host>:8080/ready`
   - 内部服务的 `/ready` 也建议验证（尤其是 `matching/order/clearing/marketdata`，其 ready 会包含 Streams 消费循环活性）

## 监控（可选，但生产强烈建议）

- 启动（默认不暴露端口，仅内网可达）：
  - `cp deploy/prod/monitoring.env.example deploy/prod/monitoring.env`
  - 填写 `GRAFANA_ADMIN_PASSWORD`
  - `docker compose -f deploy/prod/docker-compose.monitoring.yml --env-file deploy/prod/monitoring.env up -d`
  - 注意：该监控 compose 依赖 `exchange-prod-net`；先启动应用 compose（或手动 `docker network create exchange-prod-net`）

如果你启用了 `METRICS_TOKEN`：
- 将 token 写入 `deploy/prod/metrics.token`（该文件已被 gitignore）
- 在 `deploy/prod/prometheus.yml` 里取消注释 `bearer_token_file`

## 快速回滚（最小动作）

- 执行：`APP_VERSION=<previous-tag> bash deploy/prod/rollback.sh`
- 脚本会做 preflight 并按指定 tag 执行 image-only `up -d`
- 无侵入演练：`DRY_RUN=true APP_VERSION=<previous-tag> bash deploy/prod/rollback.sh`

## 生产反代/TLS（建议）

建议使用云 LB / Nginx / Caddy 做 TLS 终止与限流：
- `api.example.com` → `gateway:8080`
- `ws.example.com` → `gateway:8090`（私有推送：`/ws/private`）
-（可选）`mdws.example.com` → `marketdata:8094`

如果你暴露 `marketdata` 的 public WS（浏览器会带 `Origin`）：
- 必须设置 `MARKETDATA_WS_ALLOW_ORIGINS`（禁止 `*`）
- 建议保持 `MARKETDATA_WS_MAX_SUBSCRIPTIONS` 默认值，防止恶意订阅导致内存增长
