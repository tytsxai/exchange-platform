package service

import (
	"context"
	"errors"
	"testing"

	commondecimal "github.com/exchange/common/pkg/decimal"
	"github.com/exchange/order/internal/repository"
)

type mockConfigStore struct {
	cfg *repository.SymbolConfig
	err error
}

func (m *mockConfigStore) GetSymbolConfig(_ context.Context, _ string) (*repository.SymbolConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cfg, nil
}

type mockMatchingClient struct {
	price int64
	err   error
	calls int
}

func (m *mockMatchingClient) GetLastPrice(_ string) (int64, error) {
	m.calls++
	if m.err != nil {
		return 0, m.err
	}
	return m.price, nil
}

func TestPriceValidate_BuySellLimits(t *testing.T) {
	refPrice := int64(100 * 1e8)
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: "0.05"},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{price: refPrice}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(106*1e8)); !errors.Is(err, errPriceOutOfRange) {
		t.Fatalf("expected price out of range, got %v", err)
	}
	if err := validator.ValidatePrice("BTCUSDT", "SELL", int64(94*1e8)); !errors.Is(err, errPriceOutOfRange) {
		t.Fatalf("expected price out of range, got %v", err)
	}
	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(104*1e8)); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := validator.ValidatePrice("BTCUSDT", "SELL", int64(96*1e8)); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestPriceValidate_DefaultLimitRate(t *testing.T) {
	refPrice := int64(100 * 1e8)
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: ""},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{price: refPrice}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.02"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(103*1e8)); !errors.Is(err, errPriceOutOfRange) {
		t.Fatalf("expected price out of range, got %v", err)
	}
}

func TestPriceValidate_Disabled(t *testing.T) {
	refPrice := int64(100 * 1e8)
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: "0.01"},
	}
	matching := &mockMatchingClient{price: refPrice}
	validator := NewPriceValidator(store, matching, PriceValidatorConfig{
		Enabled:          false,
		DefaultLimitRate: *commondecimal.MustNew("0.01"),
	})
	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(1000*1e8)); err != nil {
		t.Fatalf("expected nil when disabled, got %v", err)
	}
	if matching.calls != 0 {
		t.Fatalf("expected matching not called when disabled, got %d", matching.calls)
	}
}

func TestPriceValidate_BoundaryValues(t *testing.T) {
	refPrice := int64(100 * 1e8)
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: "0.05"},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{price: refPrice}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(105*1e8)); err != nil {
		t.Fatalf("expected nil at upper boundary, got %v", err)
	}
	if err := validator.ValidatePrice("BTCUSDT", "SELL", int64(95*1e8)); err != nil {
		t.Fatalf("expected nil at lower boundary, got %v", err)
	}
}

func TestPriceValidate_ReferencePriceError(t *testing.T) {
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: "0.05"},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{err: errors.New("match down")}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(100*1e8)); err == nil {
		t.Fatal("expected error when matching fails")
	}
}

func TestPriceValidate_InvalidLimitRateUsesDefault(t *testing.T) {
	refPrice := int64(100 * 1e8)
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: "-0.1"},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{price: refPrice}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.02"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(103*1e8)); !errors.Is(err, errPriceOutOfRange) {
		t.Fatalf("expected out of range with default limit rate, got %v", err)
	}
}

func TestPriceValidate_InvalidSide(t *testing.T) {
	refPrice := int64(100 * 1e8)
	store := &mockConfigStore{
		cfg: &repository.SymbolConfig{PriceLimitRate: "0.05"},
	}
	validator := NewPriceValidator(store, &mockMatchingClient{price: refPrice}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "HOLD", int64(100*1e8)); err == nil || err.Error() != "INVALID_SIDE" {
		t.Fatalf("expected invalid side error, got %v", err)
	}
}

func TestPriceValidate_ConfigError(t *testing.T) {
	store := &mockConfigStore{err: errors.New("config error")}
	validator := NewPriceValidator(store, &mockMatchingClient{}, PriceValidatorConfig{
		Enabled:          true,
		DefaultLimitRate: *commondecimal.MustNew("0.1"),
	})

	if err := validator.ValidatePrice("BTCUSDT", "BUY", int64(100*1e8)); err == nil {
		t.Fatal("expected config error")
	}
}
