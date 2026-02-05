# Monitoring Guide

Comprehensive monitoring and observability guide for OpenExchange.

## 📊 Overview

OpenExchange provides comprehensive observability through:

- **Metrics** - Quantitative measurements (Prometheus)
- **Tracing** - Request flow tracking (Jaeger)
- **Logging** - Structured event records (Zerolog)
- **Alerts** - Proactive notifications

## 📈 Prometheus Metrics

### Key Metrics

#### HTTP Server Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `http_requests_total` | Counter | Total HTTP requests |
| `http_request_duration_seconds` | Histogram | Request latency |
| `http_requests_in_flight` | Gauge | Active requests |

```go
// Example metric usage
import "github.com/prometheus/client_golang/prometheus"

var (
    httpRequests = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    httpDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )
)

func init() {
    prometheus.MustRegister(httpRequests, httpDuration)
}
```

#### Trading Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `orders_total` | Counter | Total orders created |
| `trades_total` | Counter | Total trades executed |
| `matching_latency_seconds` | Histogram | Matching engine latency |
| `orderbook_depth` | Gauge | Current order book depth |

```go
// Trading metrics
var (
    ordersTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "orders_total",
            Help: "Total number of orders",
        },
        []string{"symbol", "side", "type"},
    )

    matchingLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "matching_latency_seconds",
            Help:    "Time to match an order",
            Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
        },
    )
)
```

#### Database Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `db_connections_open` | Gauge | Open DB connections |
| `db_connections_in_use` | Gauge | In-use DB connections |
| `db_query_duration_seconds` | Histogram | Query latency |

#### Redis Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `redis_commands_total` | Counter | Redis commands executed |
| `redis_command_duration_seconds` | Histogram | Command latency |
| `redis_pool_size` | Gauge | Connection pool size |

### Accessing Metrics

```bash
# Via HTTP endpoint
curl http://localhost:8080/metrics

# With authentication
curl -H "X-Metrics-Token: your-token" http://localhost:8080/metrics
```

## 🔍 Distributed Tracing

### Trace Spans

| Span | Description |
|------|-------------|
| `order.create` | Order creation flow |
| `order.match` | Order matching process |
| `trade.settle` | Trade settlement |
| `wallet.deposit` | Deposit processing |
| `wallet.withdraw` | Withdrawal processing |

### Trace Context Propagation

```go
import "go.opentelemetry.io/otel"

func CreateOrder(ctx context.Context, req *CreateOrderRequest) (*Order, error) {
    ctx, span := tracer.Start(ctx, "order.create")
    defer span.End()

    // Add attributes
    span.SetAttributes(
        attribute.String("order.symbol", req.Symbol),
        attribute.String("order.side", req.Side),
        attribute.String("order.type", req.Type),
    )

    // Process order
    order, err := orderService.Process(ctx, req)
    if err != nil {
        span.RecordError(err)
    }

    return order, err
}
```

### Viewing Traces

Access Jaeger UI at `http://localhost:16686` to:
- Search traces by service, operation, or tags
- View detailed span timelines
- Analyze latency distributions
- Identify error patterns

## 📝 Structured Logging

### Log Format

```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "span_id": "a1b2c3d4e5f6",
  "service": "exchange-order",
  "event": "order_created",
  "order_id": "12345",
  "symbol": "BTC_USDT",
  "side": "BUY",
  "quantity": "0.5",
  "price": "50000.00"
}
```

### Log Levels

| Level | Usage |
|-------|-------|
| `debug` | Detailed debugging information |
| `info` | Normal operation events |
| `warn` | Warning conditions |
| `error` | Error conditions |
| `fatal` | Critical failures |

### Log Attributes

```go
log.Info().
    Str("order_id", order.ID.String()).
    Str("symbol", req.Symbol).
    Str("side", req.Side).
    Str("quantity", req.Quantity).
    Interface("user_id", userID).
    Msg("order created")
```

## 🚨 Alerting

### Critical Alerts

| Alert | Severity | Threshold | Description |
|-------|----------|-----------|-------------|
| `HighErrorRate` | Critical | > 5% errors/5min | Unusual error rate detected |
| `MatchingDown` | Critical | Matching unavailable | Matching engine not responding |
| `DatabaseDown` | Critical | DB unavailable | Database connection failed |
| `OrderLatencyHigh` | Warning | p99 > 1s | High order processing latency |

### Alert Rules

```yaml
groups:
  - name: exchange-critical
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate on {{ $labels.service }}"
          description: "Error rate is {{ $value | humanizePercentage }}"

      - alert: MatchingEngineDown
        expr: up{service="matching"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Matching engine is down"

      - alert: OrderLatencyHigh
        expr: histogram_quantile(0.99, rate(http_request_duration_seconds[5m])) > 1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High order latency detected"
```

## 📊 Grafana Dashboards

### Pre-built Dashboards

1. **Overview Dashboard** - System health at a glance
2. **Trading Dashboard** - Order and trade metrics
3. **Database Dashboard** - PostgreSQL performance
4. **Redis Dashboard** - Cache and queue metrics
5. **Service Dashboard** - Per-service metrics

### Dashboard Access

```bash
# Access Grafana
open http://localhost:3000

# Default credentials
Username: admin
Password: admin123
```

### Key Dashboard Panels

```
Trading Overview:
├── Orders/min (real-time)
├── Trades/min (real-time)
├── Average Fill Rate
├── Matching Latency (p50, p95, p99)
├── Order Book Depth (by symbol)
└── Queue Backlog

System Health:
├── CPU Usage
├── Memory Usage
├── Goroutine Count
├── GC Pauses
├── Network I/O
└── Disk I/O
```

## 🔧 Health Checks

### Check Types

| Check | Endpoint | Description |
|-------|----------|-------------|
| Liveness | `/live` | Process is running |
| Readiness | `/ready` | Ready to serve traffic |
| Health | `/health` | Full health status |

### Check Responses

```bash
# Liveness
curl http://localhost:8080/live
# Response: {"status":"ok"}

# Readiness (includes dependencies)
curl http://localhost:8080/ready
# Response: {"status":"ready","checks":{"postgres":"ok","redis":"ok"}}

# Full health
curl http://localhost:8080/health
# Response: {"status":"healthy","components":{"db":"healthy","redis":"healthy"}}
```

### Implementing Custom Checks

```go
import "github.com/exchange/common/pkg/health"

func main() {
    h := health.New()

    // Register database check
    h.Register(health.NewPostgresChecker(db))

    // Register Redis check
    h.Register(health.NewRedisChecker(redis))

    // Custom check
    h.Register(health.CheckFunc(func(ctx context.Context) error {
        if !matchingEngine.IsHealthy() {
            return errors.New("matching engine not healthy")
        }
        return nil
    }))

    http.HandleFunc("/ready", h.ReadyHandler())
}
```

## 📈 Performance Monitoring

### Key Performance Indicators

| KPI | Target | Description |
|-----|--------|-------------|
| Order Latency p50 | < 50ms | 50th percentile order processing |
| Order Latency p99 | < 200ms | 99th percentile order processing |
| Matching Throughput | > 10K/sec | Orders matched per second |
| System Uptime | > 99.9% | Overall system availability |
| Error Rate | < 0.1% | Percentage of failed requests |

### Monitoring Tools

```bash
# View Prometheus targets
curl http://localhost:9090/api/v1/targets

# Query specific metric
curl http://localhost:9090/api/v1/query?query=orders_total

# View active alerts
curl http://localhost:9090/api/v1/alerts
```

## 🔒 Security Monitoring

### Security Metrics

| Metric | Description |
|--------|-------------|
| `auth_failures_total` | Authentication failures |
| `rate_limit_exceeded_total` | Rate limit violations |
| `suspicious_activity_total` | Suspicious patterns detected |

### Audit Logging

All security-relevant events are logged:

```json
{
  "event": "user.login",
  "user_id": "12345",
  "ip": "192.168.1.100",
  "user_agent": "Mozilla/5.0",
  "success": true,
  "reason": ""
}
```

## 🚀 Getting Started

### Development Environment

All monitoring tools are available via Docker Compose:

```bash
# Start infrastructure with monitoring
docker compose -f exchange-common/docker-compose.yml up -d

# Access:
# - Prometheus: http://localhost:9090
# - Grafana: http://localhost:3000
# - Jaeger: http://localhost:16686
```

### Production Setup

1. Configure external Prometheus/Grafana
2. Set up alertmanager for notifications
3. Configure log aggregation
4. Set up distributed tracing collector
5. Define SLOs and SLIs
