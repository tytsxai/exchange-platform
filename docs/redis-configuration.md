# Redis Configuration

Complete guide to Redis Streams, Pub/Sub, and caching configuration.

## 📋 Table of Contents

- [Overview](#overview)
- [Stream Configuration](#stream-configuration)
- [Pub/Sub Channels](#pubsub-channels)
- [Connection Settings](#connection-settings)
- [Performance Tuning](#performance-tuning)
- [Security](#security)
- [Monitoring](#monitoring)

---

## Overview

OpenExchange uses Redis for:

1. **Streams** - Event queue for async communication
2. **Pub/Sub** - Real-time private notifications
3. **Caching** - Market data, ticker, recent trades
4. **Rate Limiting** - Request throttling

### Default Ports

| Service | Port | Description |
|---------|------|-------------|
| Redis | 6380 | Main Redis instance |
| Redis Sentinel | 26379 | Sentinel for HA (optional) |
| Redis Cluster | 6379-6382 | Cluster mode (optional) |

---

## Stream Configuration

### Stream Names

| Stream | Default | Purpose |
|--------|---------|---------|
| Order Stream | `exchange:orders` | Order submissions |
| Event Stream | `exchange:events` | Core events (trades, updates) |
| Dead Letter Queue | `exchange:events:dlq` | Failed event processing |

### Consumer Groups

| Consumer Group | Consumer Name | Service |
|----------------|---------------|---------|
| `matching-orders` | `{service-name}-{instance-id}` | Matching Engine |
| `clearing-events` | `{service-name}-{instance-id}` | Clearing Service |
| `market-events` | `{service-name}-{instance-id}` | Market Data Service |

### Environment Variables

```bash
# Stream Configuration
ORDER_STREAM=exchange:orders
ORDER_CONSUMER_GROUP=matching-orders
ORDER_CONSUMER_NAME=matching-1

EVENT_STREAM=exchange:events
EVENT_CONSUMER_GROUP=clearing-events
EVENT_CONSUMER_NAME=clearing-1

# Dead Letter Queue
DLQ_STREAM=exchange:events:dlq
```

### Stream Configuration Example

```go
type StreamConfig struct {
    Name         string `env:"ORDER_STREAM" envDefault:"exchange:orders"`
    Group        string `env:"ORDER_CONSUMER_GROUP" envDefault:"matching-orders"`
    ConsumerName string `env:"ORDER_CONSUMER_NAME"`
    MaxLen       int64  `env:"STREAM_MAX_LENGTH" envDefault:"100000"`
    MinIdleTime  int64  `env:"STREAM_MIN_IDLE_MS" envDefault:"120000"` // 2 minutes
}
```

### Stream Commands

```bash
# View stream info
XINFO STREAM exchange:orders

# View consumer groups
XINFO GROUPS exchange:events

# View pending messages
XPENDING exchange:events clearing-events

# Read from stream (blocking)
XREADGROUP GROUP clearing-events clearing-1 BLOCK 5000 STREAMS exchange:events >

# Trim stream length
XTRIM exchange:events MAXLEN ~ 100000
```

---

## Pub/Sub Channels

### Channel Naming

| Channel Type | Pattern | Purpose |
|--------------|---------|---------|
| Private User | `private:user:{userId}:events` | User-specific notifications |
| Market Data | `market:{symbol}:updates` | Order book updates |
| System | `system:notifications` | Admin notifications |

### Private User Channels

```
private:user:{userId}:events
```

**Subscribed by:** Gateway service for WebSocket forwarding

**Event Types:**
- `order.update` - Order status changes
- `trade.created` - Trade executions
- `balance.update` - Balance changes

### Configuration

```bash
PRIVATE_USER_EVENT_CHANNEL=private:user:{userId}:events
MARKET_CHANNEL_PREFIX=market:
SYSTEM_CHANNEL=system:notifications
```

### Pub/Sub Commands

```bash
# Subscribe to channel
SUBSCRIBE private:user:12345:events

# Publish event
PUBLISH private:user:12345:events '{"type":"order.update","data":{...}}'

# List channels
PUBSUB CHANNELS private:user:*
```

---

## Connection Settings

### Connection Pool

```go
type RedisConfig struct {
    Addr         string `env:"REDIS_ADDR" envDefault:"localhost:6380"`
    Password     string `env:"REDIS_PASSWORD"`
    DB           int    `env:"REDIS_DB" envDefault:"0"`
    PoolSize     int    `env:"REDIS_POOL_SIZE" envDefault:"50"`
    MinIdleConns int    `env:"REDIS_MIN_IDLE_CONNS" envDefault:"10"`
    DialTimeout  int    `env:"REDIS_DIAL_TIMEOUT" envDefault:"5"`
    ReadTimeout  int    `env:"REDIS_READ_TIMEOUT" envDefault:"3"`
    WriteTimeout int    `env:"REDIS_WRITE_TIMEOUT" envDefault:"3"`
}
```

### TLS Configuration

```go
type TLSConfig struct {
    Enabled     bool   `env:"REDIS_TLS" envDefault:"false"`
    CACertFile  string `env:"REDIS_CACERT"`
    CertFile    string `env:"REDIS_CERT"`
    KeyFile     string `env:"REDIS_KEY"`
    ServerName  string `env:"REDIS_SERVER_NAME"`
}
```

### Client Initialization

```go
import "github.com/redis/go-redis/v9"

func NewRedisClient(cfg *RedisConfig) *redis.Client {
    return redis.NewClient(&redis.Options{
        Addr:         cfg.Addr,
        Password:      cfg.Password,
        DB:            cfg.DB,
        PoolSize:      cfg.PoolSize,
        MinIdleConns:  cfg.MinIdleConns,
        DialTimeout:   time.Duration(cfg.DialTimeout) * time.Second,
        ReadTimeout:   time.Duration(cfg.ReadTimeout) * time.Second,
        WriteTimeout:  time.Duration(cfg.WriteTimeout) * time.Second,
    })
}
```

---

## Performance Tuning

### Memory Optimization

```bash
# Set max memory
CONFIG SET maxmemory 4gb

# Eviction policy
CONFIG SET maxmemory-policy allkeys-lru

# Enable AOF persistence
CONFIG SET appendonly yes
CONFIG SET appendfsync everysec
```

### Slow Log

```bash
# Enable slow log
CONFIG SET slowlog-log-slower-than 1000  # 1ms threshold
CONFIG SET slowlog-max-len 1000            # Keep last 1000 entries

# View slow log
SLOWLOG GET 10
```

### Latency Monitoring

```bash
# Monitor commands in real-time
MONITOR

# Check command stats
INFO commandstats
```

---

## Security

### Authentication

```bash
# Require password
CONFIG SET requirepass your-secure-password

# ACL configuration (Redis 6+)
ACL SETUSER default on >yourpassword ~* +@all
```

### Network Security

```bash
# Bind to specific interface
CONFIG SET bind 127.0.0.1

# Disable dangerous commands
ACL DELUSER nosuchuser 2>/dev/null || true
CONFIG SET rename-command CONFIG ""
CONFIG SET rename-command FLUSHDB ""
CONFIG SET rename-command FLUSHALL ""
```

### TLS Encryption

```bash
# Enable TLS
tls-port 6380
port 0
tls-cert-file /path/to/cert.pem
tls-key-file /path/to/key.pem
tls-ca-cert-file /path/to/ca.pem
```

---

## Monitoring

### Key Metrics

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| `used_memory` | > 80% maxmemory | Memory pressure |
| `connected_clients` | > 1000 | Connection count |
| `instantaneous_ops_per_sec` | < 100 | Low throughput |
| `rejected_connections` | > 0 | Connection rejections |
| `keyspace_hits_ratio` | < 0.5 | Cache hit rate |

### Prometheus Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    redisCommands = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "redis_commands_total",
            Help: "Total Redis commands",
        },
        []string{"command", "status"},
    )

    redisLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "redis_command_duration_seconds",
            Help:    "Redis command latency",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
        },
        []string{"command"},
    )
)
```

### Health Check

```bash
# PING command
redis-cli -h localhost -p 6380 PING
# Response: PONG

# Check memory
redis-cli -h localhost -p 6380 INFO memory | grep used_memory

# Check connected clients
redis-cli -h localhost -p 6380 INFO clients | grep connected_clients
```

---

## High Availability

### Sentinel Configuration

```bash
# sentinel.conf
port 26379
sentinel monitor mymaster redis-master 6380 2
sentinel down-after-milliseconds mymaster 30000
sentinel failover-timeout mymaster 180000
sentinel parallel-syncs mymaster 1
```

### Cluster Configuration

```bash
# redis-cluster.conf
cluster-enabled yes
cluster-config-file nodes.conf
cluster-node-timeout 15000
cluster-migration-barrier 1
```

---

## 📖 Related Documentation

- [Architecture](architecture.md) - System architecture
- [Event Model](event-model.md) - Event-driven communication
- [Monitoring](monitoring.md) - Observability setup
