# 交易所项目验证与测试开发计划

## 概述
- **目标**: 完成交易所系统的验证与测试，确保8个微服务正常运行，单元测试覆盖率≥90%
- **当前状态**: 服务代码已完成，无单元测试文件，有E2E脚本但未验证

## 任务分解

### 阶段1: 基础设施启动（串行依赖）

#### TASK-001: Docker Compose 环境启动
- **描述**: 启动 PostgreSQL 和 Redis 容器
- **文件范围**: `exchange-common/docker-compose.yml`
- **依赖**: 无
- **测试命令**: `docker-compose -f exchange-common/docker-compose.yml ps`
- **验收标准**: postgres 和 redis 容器状态为 healthy

#### TASK-002: 数据库 Schema 初始化
- **描述**: 执行数据库初始化脚本创建所有表结构
- **文件范围**: `exchange-common/scripts/init-db.sql`
- **依赖**: TASK-001
- **测试命令**: `docker-compose -f exchange-common/docker-compose.yml exec -T postgres psql -U exchange -d exchange -c '\dt exchange_*.*'`
- **验收标准**: 所有 schema 和表创建成功

#### TASK-003: 8个微服务启动验证
- **描述**: 启动所有微服务并验证健康检查端点
- **文件范围**:
  - `exchange-gateway/cmd/gateway/main.go` (端口 8080)
  - `exchange-order/cmd/order/main.go` (端口 8081)
  - `exchange-matching/cmd/matching/main.go` (端口 8082)
  - `exchange-clearing/cmd/clearing/main.go` (端口 8083)
  - `exchange-marketdata/cmd/marketdata/main.go` (端口 8084)
  - `exchange-user/cmd/user/main.go` (端口 8085)
  - `exchange-admin/cmd/admin/main.go` (端口 8086)
  - `exchange-wallet/cmd/wallet/main.go` (端口 8087)
- **依赖**: TASK-002
- **测试命令**:
  ```bash
  for port in 8080 8081 8082 8083 8084 8085 8086 8087; do
    curl -sf http://localhost:$port/health || echo "Service on $port failed"
  done
  ```
- **验收标准**: 所有8个服务 /health 返回 200

#### TASK-004: E2E 测试脚本执行
- **描述**: 运行端到端测试验证核心业务流程
- **文件范围**: `exchange-common/scripts/e2e-test.sh`
- **依赖**: TASK-003
- **测试命令**: `bash exchange-common/scripts/e2e-test.sh`
- **验收标准**: 所有E2E测试用例通过

### 阶段2: 单元测试补齐（可并行）

#### TASK-005: exchange-common 单元测试
- **描述**: 为公共组件编写单元测试
- **文件范围**:
  - `exchange-common/pkg/redis/stream.go` → `stream_test.go`
  - `exchange-common/pkg/auth/*.go` → `*_test.go`
  - `exchange-common/pkg/validator/*.go` → `*_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-common && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-006: exchange-user 单元测试
- **描述**: 为用户服务编写单元测试
- **文件范围**:
  - `exchange-user/internal/service/user.go` → `user_test.go`
  - `exchange-user/internal/repository/user.go` → `user_test.go`
  - `exchange-user/internal/handler/*.go` → `*_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-user && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-007: exchange-order 单元测试
- **描述**: 为订单服务编写单元测试
- **文件范围**:
  - `exchange-order/internal/service/order.go` → `order_test.go`
  - `exchange-order/internal/repository/order.go` → `order_test.go`
  - `exchange-order/internal/handler/*.go` → `*_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-order && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-008: exchange-matching 单元测试
- **描述**: 为撮合引擎编写单元测试
- **文件范围**:
  - `exchange-matching/internal/engine/*.go` → `*_test.go`
  - `exchange-matching/internal/orderbook/*.go` → `*_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-matching && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-009: exchange-clearing 单元测试
- **描述**: 为清算服务编写单元测试
- **文件范围**:
  - `exchange-clearing/internal/service/clearing.go` → `clearing_test.go`
  - `exchange-clearing/internal/repository/balance.go` → `balance_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-clearing && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-010: exchange-wallet 单元测试
- **描述**: 为钱包服务编写单元测试
- **文件范围**:
  - `exchange-wallet/internal/service/wallet.go` → `wallet_test.go`
  - `exchange-wallet/internal/repository/wallet.go` → `wallet_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-wallet && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-011: exchange-admin 单元测试
- **描述**: 为管理服务编写单元测试
- **文件范围**:
  - `exchange-admin/internal/service/admin.go` → `admin_test.go`
  - `exchange-admin/internal/repository/admin.go` → `admin_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-admin && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-012: exchange-marketdata 单元测试
- **描述**: 为行情服务编写单元测试
- **文件范围**:
  - `exchange-marketdata/internal/service/*.go` → `*_test.go`
  - `exchange-marketdata/internal/handler/*.go` → `*_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-marketdata && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

#### TASK-013: exchange-gateway 单元测试
- **描述**: 为网关服务编写单元测试
- **文件范围**:
  - `exchange-gateway/internal/middleware/*.go` → `*_test.go`
  - `exchange-gateway/internal/handler/*.go` → `*_test.go`
- **依赖**: 无
- **测试命令**: `cd exchange-gateway && go test ./... -coverprofile=coverage.out -covermode=atomic`
- **验收标准**: 覆盖率≥90%

## 任务依赖图

```
TASK-001 (Docker)
    ↓
TASK-002 (Schema)
    ↓
TASK-003 (Services)
    ↓
TASK-004 (E2E)

并行执行:
TASK-005 ─┬─ TASK-006 ─┬─ TASK-007 ─┬─ TASK-008
TASK-009 ─┴─ TASK-010 ─┴─ TASK-011 ─┴─ TASK-012 ─┬─ TASK-013
```

## UI 判断
- **needs_ui**: false
- **evidence**: 项目为纯后端微服务架构，未发现 .tsx/.jsx/.vue 组件文件或 .css/.scss 样式文件

## 测试策略

### 单元测试要求
1. **Happy Path**: 覆盖所有正常业务流程
2. **Edge Cases**: 边界值、空输入、最大限制
3. **Error Handling**: 无效输入、失败场景、权限错误
4. **State Transitions**: 订单状态流转、余额变更

### Mock 策略
- 数据库层: 使用接口抽象 + mock 实现
- Redis: 使用 miniredis 或接口 mock
- HTTP 客户端: 使用 httptest.Server

## 执行顺序建议
1. 先执行 TASK-001 ~ TASK-004 验证系统可运行
2. 并行执行 TASK-005 ~ TASK-013 补齐单元测试
3. 汇总覆盖率报告，确保每个服务≥90%
