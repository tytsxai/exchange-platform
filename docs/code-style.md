# Code Style Guide

Coding conventions and best practices for OpenExchange development.

## 📋 Go Conventions

### Naming

| Element | Convention | Example |
|---------|------------|---------|
| Package | lowercase, short | `order`, `matching` |
| Variable | CamelCase | `orderID`, `totalBalance` |
| Constant | CamelCase, uppercase | `MaxOrderSize`, `DefaultFee` |
| Function | Verb-first, CamelCase | `CreateOrder`, `GetBalance` |
| Type | CamelCase, noun | `OrderService`, `TradeEvent` |
| Interface | -er suffix | `Repository`, `Service` |
| Error | `Err` prefix | `ErrInsufficientBalance` |

### Package Structure

```
exchange-order/
├── cmd/
│   └── order/
│       └── main.go           # Application entrypoint
├── internal/
│   ├── config/               # Configuration
│   ├── handler/              # HTTP/gRPC handlers
│   ├── repository/           # Database operations
│   ├── service/              # Business logic
│   └── types/                # Domain types
├── pkg/                      # Shared utilities
└── api/                      # OpenAPI specs
```

### Error Handling

**DO:**

```go
// Return errors with context
if err := db.QueryRowContext(ctx, query, id).Scan(&order); err != nil {
    return nil, fmt.Errorf("failed to get order %s: %w", id, err)
}

// Use sentinel errors for known conditions
if err == sql.ErrNoRows {
    return nil, ErrOrderNotFound
}
```

**DON'T:**

```go
// Don't ignore errors
order, _ := db.GetOrder(id)

// Don't shadow errors
if err := doSomething(); err != nil {
    return err // ❌ Lost context
}

// Don't panic
if x < 0 {
    panic("negative value") // ❌
}
```

### Context Usage

**DO:**

```go
func CreateOrder(ctx context.Context, req *OrderRequest) (*Order, error) {
    ctx, span := tracer.Start(ctx, "order.create")
    defer span.End()

    // Pass context to all operations
    order, err := s.repo.Create(ctx, req)
    if err != nil {
        span.RecordError(err)
        return nil, err
    }

    return order, nil
}
```

**DON'T:**

```go
// Don't use background for long operations
order, err := s.repo.Create(context.Background(), req) // ❌

// Don't pass nil context
order, err := s.repo.Create(nil, req) // ❌
```

### Struct Definitions

```go
// Use field tags for serialization
type Order struct {
    ID        snowflake.ID  `json:"id" db:"id"`
    Symbol    string        `json:"symbol" db:"symbol"`
    Side      Side          `json:"side" db:"side"`
    Type      OrderType     `json:"type" db:"type"`
    Quantity  decimal.Decimal `json:"quantity" db:"quantity"`
    Price     decimal.Decimal `json:"price" db:"price"`
    Status    OrderStatus   `json:"status" db:"status"`
    CreatedAt time.Time     `json:"created_at" db:"created_at"`
}

// Group related fields
type Trade struct {
    ID            snowflake.ID  `json:"id"`
    Symbol        string        `json:"symbol"`
    Price         decimal.Decimal `json:"price"`
    Quantity      decimal.Decimal `json:"quantity"`
    MakerOrderID  snowflake.ID  `json:"maker_order_id"`
    TakerOrderID  snowflake.ID  `json:"taker_order_id"`
    Fee           decimal.Decimal `json:"fee"`
    FeeCurrency   string        `json:"fee_currency"`
    CreatedAt     time.Time     `json:"created_at"`
}
```

### Interfaces

```go
// Define interfaces where used (avoid upfront interfaces)
type OrderRepository interface {
    Create(ctx context.Context, order *Order) error
    GetByID(ctx context.Context, id snowflake.ID) (*Order, error)
    UpdateStatus(ctx context.Context, id snowflake.ID, status OrderStatus) error
    ListByUser(ctx context.Context, userID string, limit, offset int) ([]*Order, error)
}
```

## 🔧 Configuration

### Environment Variables

```go
type Config struct {
    // Required fields
    DatabaseURL string `env:"DATABASE_URL,required"`
    RedisAddr   string `env:"REDIS_ADDR,required"`

    // Optional with defaults
    HTTPPort    int    `env:"HTTP_PORT" envDefault:"8080"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    // Sensitive (use secrets manager in production)
    APISecret   string `env:"API_SECRET"`
}
```

## 📝 Documentation

### Comments

```go
// CreateOrder creates a new order for the specified trading pair.
// It validates the order parameters, checks the user's balance,
// freezes the required funds, and submits the order to the matching engine.
//
// The order ID is generated using a distributed snowflake algorithm
// to ensure global uniqueness across all services.
func CreateOrder(ctx context.Context, req *CreateOrderRequest) (*Order, error) {
    // Implementation
}
```

### Public API Documentation

```go
// Package order provides order management services for the exchange.
// It handles order creation, cancellation, and querying through
// a combination of REST and gRPC APIs.
//
// # Order Lifecycle
//
// Orders go through the following states:
//   - INIT: Order received but not yet validated
//   - NEW: Order validated and submitted to matching
//   - PARTIALLY_FILLED: Some quantity has been filled
//   - FILLED: Order completely filled
//   - CANCELED: Order cancelled by user or system
//
// For more details, see [Trading Flow](trading-flow.md).
package order
```

## 🧪 Testing

### Test Files

```go
// order_service_test.go
func TestCreateOrder(t *testing.T) {
    // Use table-driven tests
    tests := []struct {
        name    string
        request *CreateOrderRequest
        wantErr bool
    }{
        {
            name:    "valid order",
            request: validOrderRequest(),
            wantErr: false,
        },
        {
            name:    "insufficient balance",
            request: insufficientBalanceRequest(),
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            order, err := svc.CreateOrder(context.Background(), tt.request)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, order)
            }
        })
    }
}
```

## 🎨 Code Formatting

### Imports

```go
import (
    // Standard library
    "context"
    "fmt"
    "time"

    // External packages
    "github.com/google/uuid"
    "github.com/shopspring/decimal"

    // Internal packages
    "github.com/exchange/common/pkg/errors"
    "github.com/exchange/common/pkg/logger"
)
```

Use `go fmt` and organize imports:

```bash
# Format code
go fmt ./...

# Organize imports
go mod tidy
```

## 🔒 Security

### Sensitive Data

```go
// NEVER log sensitive data
log.Info().
    Str("user_id", userID).
    Str("action", "login") // ✅ OK

log.Info().
    Str("password", password) // ❌ NEVER
    Str("api_secret", secret) // ❌ NEVER
```

### SQL Injection Prevention

```go
// Use parameterized queries
rows, err := db.QueryContext(ctx,
    "SELECT * FROM orders WHERE user_id = $1 AND symbol = $2",
    userID, symbol,
)

// DON'T concatenate strings
query := "SELECT * FROM orders WHERE user_id = " + userID // ❌
```

## 📐 Design Principles

### Single Responsibility

```go
// ✅ Good: Separate concerns
type OrderRepository interface {
    Create(ctx context.Context, order *Order) error
    GetByID(ctx context.Context, id snowflake.ID) (*Order, error)
}

type OrderService struct {
    repo OrderRepository
    matching MatchingEngine
    clearing ClearingService
}

// ❌ Bad: God object
type BigOrderService struct {
    // 50+ methods for everything
}
```

### Dependency Injection

```go
type OrderService struct {
    repo      OrderRepository
    matching  MatchingEngine
    clearing  ClearingService
    logger    *logger.Logger
}

func NewOrderService(
    repo OrderRepository,
    matching MatchingEngine,
    clearing ClearingService,
    log *logger.Logger,
) *OrderService {
    return &OrderService{
        repo:     repo,
        matching: matching,
        clearing: clearing,
        logger:   log,
    }
}
```

## 📏 Line Length

- Maximum line length: **120 characters**
- Exception: Long URLs in comments may exceed

## 🎯 Go Modules

### go.mod Example

```go
module github.com/exchange/order

go 1.25

require (
    github.com/exchange/common v1.0.0
    github.com/google/uuid v1.4.0
    github.com/shopspring/decimal v1.3.1
)
```

## 🔄 Git Commits

### Conventional Commits

```
<type>(<scope>): <subject>

Types:
- feat: New feature
- fix: Bug fix
- docs: Documentation only
- style: Formatting changes
- refactor: Code restructuring
- test: Adding tests
- chore: Maintenance
```

Examples:

```
feat(order): add stop-limit order support
fix(matching): resolve race condition in order book
docs(api): update order creation endpoint
refactor(clearing): simplify balance calculation
test(service): add order creation unit tests
```

## 📚 Resources

- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout)
