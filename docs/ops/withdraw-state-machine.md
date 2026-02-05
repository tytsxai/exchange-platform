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
