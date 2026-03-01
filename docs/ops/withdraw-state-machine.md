# 提现状态机

## 状态定义

| 状态 | 说明 |
|------|------|
| PENDING | 待审核 |
| APPROVED | 已审核 |
| REJECTED | 已拒绝 |
| PROCESSING | 处理中 |
| COMPLETED | 已完成 |
| FAILED | 失败 |

## 状态转换

```
PENDING ──┬── Approve() ──> APPROVED ──> StartProcessing() ──> PROCESSING ──┬── Complete() ──> COMPLETED
          │                                                                  │
          └── Reject() ───> REJECTED                                         └── Fail() ─────> FAILED
```

## API 方法

| 方法 | 前置状态 | 目标状态 |
|------|----------|----------|
| `Submit()` | - | PENDING |
| `Approve()` | PENDING | APPROVED |
| `Reject(reason)` | PENDING | REJECTED |
| `StartProcessing()` | APPROVED | PROCESSING |
| `Complete()` | PROCESSING | COMPLETED |
| `Fail(reason)` | PROCESSING | FAILED |
| `Get()` | - | - |

## 异常路径说明（上线必看）

- `RequestWithdraw()` 采用“先冻结、再落库”的顺序。若冻结成功但写提现单失败：
  - 服务不会自动解冻（避免与同幂等键重试产生“提现单成功但未冻结”的错配）。
  - 推荐处理方式：使用同一 `IdempotencyKey` 重试请求，由幂等逻辑完成闭环。
  - 运维应关注日志关键字：`create withdrawal failed after freeze`，并按 runbook 执行人工核查。
