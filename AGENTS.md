# AGENTS.md

本文件是 `exchange-platform` 的架构导航与维护约定，目标是让后来者能在最短时间理解系统骨架、上下游依赖与改动边界。

## 1) 目录骨架（后端微服务）

```text
exchange-platform/
├── exchange-gateway/      # 统一入口：鉴权、限流、路由转发、私有 WS
├── exchange-order/        # 订单生命周期：下单/撤单/订单查询/撮合事件回写
├── exchange-matching/     # 撮合引擎：消费订单流、生成成交/订单状态事件
├── exchange-clearing/     # 清算账本：冻结/解冻/结算/账本查询
├── exchange-marketdata/   # 行情：消费撮合事件，维护深度/成交/ticker
├── exchange-user/         # 用户与 API Key：注册登录、签名校验、风控限流
├── exchange-wallet/       # 充提与地址管理：调用清算完成资金变动
├── exchange-admin/        # 后台管理 API：交易对/风控开关/RBAC/审计
├── exchange-common/       # 公共库：错误码、鉴权、日志、追踪、健康检查等
├── deploy/prod/           # 生产部署、回滚、监控告警示例
├── docs/                  # 架构、运维、测试、上线清单
└── scripts/               # E2E、验收、数据库维护脚本
```

## 2) 核心调用链（交易主路径）

```text
Client
  -> exchange-gateway
    -> exchange-order
      -> Redis Stream(order)
        -> exchange-matching
          -> Redis Stream(event)
            -> exchange-order (订单状态回写)
            -> exchange-clearing (资金结算)
            -> exchange-marketdata (行情增量)
```

## 3) 模块职责与边界

- `exchange-gateway`
  - 只做入口能力（鉴权/权限/限流/转发），不持有业务状态。
  - 下游身份只信任网关注入头（如 `X-User-Id`）。

- `exchange-order`
  - 订单真相源（DB）与撮合输入生产者（Redis Stream）。
  - 通过消费撮合事件更新订单状态与成交记录。

- `exchange-matching`
  - 纯撮合职责（订单簿 + 事件输出），不直接改动账户资产。
  - 输出事件是后续清算/订单回写/行情更新的唯一输入。

- `exchange-clearing`
  - 资金账本真相源（`account_balances` + `ledger_entries`）。
  - 所有余额变化必须走账本分录，保持可追溯。

- `exchange-marketdata`
  - 仅维护面向查询/推送的衍生状态（深度、成交、ticker）。
  - 不得反向写入交易或资金真相源。

- `exchange-user`
  - 用户身份、API Key、签名校验与登录防爆破。
  - 签名与权限是网关放行前置条件。

- `exchange-wallet`
  - 充提流程编排；资产变动最终落到 clearing。
  - 链上扫描属于可替换组件，不应突破资金边界。

- `exchange-admin`
  - 面向运营治理，不参与撮合核心路径。
  - 高风险操作必须有审计留痕。

## 4) 近期关键补强（2026-02）

- 网关：私有 API 与私有 WS 增加 API Key 权限位强校验（READ/TRADE）。
- 网关：用户限流顺序调整为 `Auth -> UserRateLimit`，避免退化为按 IP 限流。
- 行情：深度维护从“简化占位”升级为按订单生命周期增量更新（接受/部分成交/撤单/成交通知）。
- 清算：去除 `USDT + 1e8` 硬编码，按交易对配置解析资产与数量精度计算 `quoteQty`。
- 清算：余额首写并发唯一键冲突转为乐观锁重试语义，降低偶发失败。
- Admin：`/admin/*` 在 HTTP 入口新增 RBAC 权限中间件（按 method + route 强制校验，未配置规则默认拒绝）。
- Wallet：提现状态更新升级为 CAS（`expected_status -> target_status`），防并发覆盖导致状态错乱。

## 5) 维护规则（简版）

- 任何跨服务接口变更必须同步更新：
  - `contracts/` 契约
  - `docs/` 对应文档
  - 相关 E2E/集成测试
- 任何资金相关逻辑改动，必须附带：
  - 幂等设计说明
  - 异常回滚/补偿路径
  - 至少一个失败场景测试
