# 默认生产部署方案（平衡版）

你说“先别纠结形态，让我默认选一个最佳的”，这里给出一个**能尽快落地、易运维、后续可迁移**的默认方案：

## 选择

- **应用**：Docker Compose（单机/少量机器）部署 8 个 Go 服务
- **基础设施**：生产默认使用**托管 Postgres + 托管 Redis**（TLS + 密码）
- **对外暴露**：
  - `exchange-gateway`（HTTP 8080 + WS 8090）
  - （可选）`exchange-marketdata` 的 public WS 8094：需要你明确是否要对外；默认建议走内网或经反代限流
- **不对外暴露**：`order/matching/clearing/user/admin/wallet` 的 HTTP 端口（仅内网可达）

## 为什么这样选（“平衡”）

- 不引入 Kubernetes 的复杂度，但具备容器化发布与快速回滚能力（换镜像/版本即可）
- 将最难运维的组件（DB/Redis）交给托管服务，降低数据丢失与维护成本
- 通过网络边界 + 内部 token，把“误暴露”风险降到可控

## 你需要做的最小动作

1. 准备生产环境变量：
   - 复制 `deploy/prod/prod.env.example` → `deploy/prod/prod.env`，填入真实值
2. 运行预检（会 fast-fail 防止误配）：
   - `bash exchange-common/scripts/prod-preflight.sh`
3. 部署：
   - `docker compose -f deploy/prod/docker-compose.yml --env-file deploy/prod/prod.env up -d --build`
4. 验证：
   - `curl -sf http://<gateway-host>:8080/health`
   - `curl -sf http://<gateway-host>:8080/ready`

## 生产反代/TLS（建议）

建议使用云 LB / Nginx / Caddy 做 TLS 终止与限流：
- `api.example.com` → `gateway:8080`
- `ws.example.com` → `gateway:8090`
-（可选）`mdws.example.com` → `marketdata:8094`

