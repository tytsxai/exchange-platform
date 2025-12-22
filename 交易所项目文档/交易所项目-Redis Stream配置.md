---
title: 交易所项目-Redis Stream 配置
summary: Redis Stream 与私有推送通道命名约定与环境变量说明。
created: 2025-12-21
updated: 2025-12-21
stage: draft
visibility: internal
owner: me
ai_include: true
---

# 交易所项目-Redis Stream 配置

本项目统一使用以下 Redis Stream/通道命名，并在所有服务的 `config.go` 中提供默认配置。

## 统一名称

- 订单流: `exchange:orders`
- 事件流: `exchange:events`
- 私有推送: `private:user:{userId}:events`

## 环境变量

- `ORDER_STREAM`: 订单流名称，默认 `exchange:orders`
- `EVENT_STREAM`: 事件流名称，默认 `exchange:events`
- `PRIVATE_USER_EVENT_CHANNEL`: 私有推送通道模板，默认 `private:user:{userId}:events`

## 服务约定

- `exchange-order`
  - 发送订单到 `ORDER_STREAM`
  - 消费撮合事件从 `EVENT_STREAM`
  - 私有推送通道遵循 `PRIVATE_USER_EVENT_CHANNEL`
- `exchange-matching`
  - 消费订单从 `ORDER_STREAM`
  - 产出事件到 `EVENT_STREAM`
- `exchange-clearing`
  - 消费撮合事件从 `EVENT_STREAM`
- `exchange-marketdata`
  - 消费撮合事件从 `EVENT_STREAM`
- `exchange-gateway`
  - 订阅私有推送通道 `PRIVATE_USER_EVENT_CHANNEL`

如需调整命名，请在对应服务的环境变量中覆盖默认值。
