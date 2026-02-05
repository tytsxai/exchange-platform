# Testing Guide

Comprehensive testing guidelines for the OpenExchange platform.

## 🧪 Testing Philosophy

OpenExchange follows a multi-layered testing strategy:

1. **Unit Tests** - Individual function/method testing
2. **Integration Tests** - Service interaction testing
3. **End-to-End Tests** - Complete workflow validation
4. **Load Tests** - Performance and stress testing

## 📁 Test Structure

```
exchange-*/               # Each service
├── internal/
│   ├── service/          # Service tests
│   ├── handler/          # Handler tests
│   └── repository/       # Repository tests
├── *_test.go             # Test files
└── integration/           # Integration tests
```

## 🏃 Running Tests

### Run All Tests

```bash
# Run all tests in the project
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Run with race detection
go test -race ./...
```

### Run Tests for Specific Service

```bash
# Test a single module
go test ./exchange-order/...

# Test with verbose output
go test -v ./exchange-matching/...

# Run specific test
go test -v ./exchange-order/... -run TestOrderCreate
```

## 📝 Unit Testing Best Practices

### Table-Driven Tests

Use table-driven tests for consistent test coverage:

```go
func TestProcessOrder(t *testing.T) {
    tests := []struct {
        name        string
        order       *Order
        wantErr     bool
        wantStatus  OrderStatus
    }{
        {
            name:       "valid limit order",
            order:      createTestLimitOrder(),
            wantErr:    false,
            wantStatus: OrderStatusNew,
        },
        {
            name:       "invalid price",
            order:      createTestOrderWithInvalidPrice(),
            wantErr:    true,
            wantStatus: OrderStatusInit,
        },
        {
            name:       "insufficient balance",
            order:      createTestOrderWithInsufficientBalance(),
            wantErr:    true,
            wantStatus: OrderStatusInit,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ProcessOrder(tt.order)
            if (err != nil) != tt.wantErr {
                t.Errorf("ProcessOrder() error = %v, wantErr %v", err, tt.wantErr)
            }
            if tt.order.Status != tt.wantStatus {
                t.Errorf("ProcessOrder() status = %v, want %v", tt.order.Status, tt.wantStatus)
            }
        })
    }
}
```

### Mock Dependencies

Use interfaces for testable code:

```go
// Define interface for dependencies
type BalanceRepository interface {
    GetBalance(userID string, asset string) (*Balance, error)
    Freeze(userID string, asset string, amount string) error
}

// Mock implementation for testing
type MockBalanceRepository struct {
    balances map[string]*Balance
}

func (m *MockBalanceRepository) GetBalance(userID string, asset string) (*Balance, error) {
    key := fmt.Sprintf("%s:%s", userID, asset)
    return m.balances[key], nil
}
```

## 🔄 Integration Testing

### Database Integration Tests

```go
func TestOrderRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Setup test database
    db, cleanup := setupTestDB(t)
    defer cleanup()

    repo := NewOrderRepository(db)

    // Create test order
    order := &Order{
        ID:       snowflake.MustNextID(),
        Symbol:   "BTC_USDT",
        Side:     SideBuy,
        Type:     OrderTypeLimit,
        Quantity: "0.5",
        Price:    "50000.00",
    }

    // Test CRUD operations
    err := repo.Create(order)
    require.NoError(t, err)

    found, err := repo.GetByID(order.ID)
    require.NoError(t, err)
    assert.Equal(t, order.Symbol, found.Symbol)

    err = repo.UpdateStatus(order.ID, OrderStatusFilled)
    require.NoError(t, err)
}
```

### Service Integration Tests

```go
func TestOrderService_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Setup services
    orderSvc, clearingSvc, matchingSvc := setupServices(t)
    svc := NewOrderService(orderSvc, clearingSvc, matchingSvc)

    // Test order flow
    orderID, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
        Symbol:   "BTC_USDT",
        Side:     "BUY",
        Type:     "LIMIT",
        Quantity: "0.5",
        Price:    "50000.00",
    })
    require.NoError(t, err)
    assert.NotEmpty(t, orderID)
}
```

## 📊 Test Coverage Requirements

### Minimum Coverage

| Component | Minimum Coverage |
|-----------|------------------|
| Core Logic (matching, clearing) | 90% |
| Service Layer | 80% |
| Repository Layer | 70% |
| Handlers | 60% |

### Coverage Report

```bash
# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

## 🔍 Test Categories

### Fast Tests (Short Mode)

```go
func TestDecimal_Math(t *testing.T) {
    // Quick unit tests - always run
    price := decimal.MustNew("100.50")
    qty := decimal.MustNew("2")
    total := price.Mul(qty)
    assert.Equal(t, "201.00", total.String())
}
```

### Integration Tests

```go
func TestMatchingEngine_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Requires full environment")
    }
    // Full integration tests
}
```

## 🐛 Test Fixtures

### Common Test Data

```go
// fixtures/test_data.go
package fixtures

func TestUser() *User {
    return &User{
        ID:       snowflake.MustNextID(),
        Email:    "test@example.com",
        Status:   UserStatusActive,
        CreatedAt: time.Now(),
    }
}

func TestOrder() *Order {
    return &Order{
        ID:       snowflake.MustNextID(),
        Symbol:   "BTC_USDT",
        Side:     SideBuy,
        Type:     OrderTypeLimit,
        Quantity: "1.0",
        Price:    "50000.00",
        Status:   OrderStatusNew,
    }
}

func TestTrade() *Trade {
    return &Trade{
        ID:        snowflake.MustNextID(),
        Symbol:    "BTC_USDT",
        Price:     "50000.00",
        Quantity:  "0.5",
        MakerOrderID: snowflake.MustNextID(),
        TakerOrderID: snowflake.MustNextID(),
    }
}
```

## ⚡ Performance Testing

### Benchmark Tests

```go
func BenchmarkMatchingEngine_Match(b *testing.B) {
    engine := NewMatchingEngine()

    // Setup
    for i := 0; i < 10000; i++ {
        engine.AddOrder(createRandomOrder())
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        order := createRandomOrder()
        engine.Match(order)
    }
}

func BenchmarkDecimal_Mul(b *testing.B) {
    price := decimal.MustNew("50000.123456")
    qty := decimal.MustNew("0.5")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = price.Mul(qty)
    }
}
```

## 🔄 Continuous Integration

### GitHub Actions Workflow

```yaml
name: Tests

on:
  push:
    branches: [main, master]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'

      - name: Cache dependencies
        uses: actions/cache@v3
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

      - name: Run unit tests
        run: go test -short -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
```

## 🎯 Test-Driven Development

### TDD Cycle

1. **Red**: Write a failing test
2. **Green**: Write minimal code to pass
3. **Refactor**: Improve code while keeping tests passing

### Example: Adding New Order Type

```go
// 1. Write test first
func TestStopLimitOrder(t *testing.T) {
    order := &StopLimitOrder{
        StopPrice: "51000.00",
        LimitPrice: "50900.00",
    }

    // Test stop trigger
    marketPrice := "51200.00"
    assert.True(t, order.ShouldTrigger(marketPrice))

    // Test limit order creation
    limitOrder := order.ToLimitOrder()
    assert.Equal(t, "50900.00", limitOrder.Price)
}

// 2. Implement code
type StopLimitOrder struct {
    StopPrice  string
    LimitPrice string
}

func (o *StopLimitOrder) ShouldTrigger(currentPrice string) bool {
    // Implementation
}
```

## 📋 Testing Checklist

- [ ] All public APIs have tests
- [ ] Edge cases are covered
- [ ] Error paths are tested
- [ ] Race conditions are tested
- [ ] Integration tests pass
- [ ] Performance benchmarks are stable
- [ ] Coverage meets requirements
