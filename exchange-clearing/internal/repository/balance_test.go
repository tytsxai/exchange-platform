package repository

import (
	"testing"
)

func TestBalanceStruct(t *testing.T) {
	balance := &Balance{
		UserID:    123,
		Asset:     "USDT",
		Available: 10000,
		Frozen:    500,
		Version:   1,
		UpdatedAt: 1000,
	}

	if balance.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", balance.UserID)
	}
	if balance.Asset != "USDT" {
		t.Fatalf("expected Asset=USDT, got %s", balance.Asset)
	}
	if balance.Available != 10000 {
		t.Fatalf("expected Available=10000, got %d", balance.Available)
	}
	if balance.Frozen != 500 {
		t.Fatalf("expected Frozen=500, got %d", balance.Frozen)
	}
}

func TestLedgerEntryStruct(t *testing.T) {
	entry := &LedgerEntry{
		LedgerID:       1,
		IdempotencyKey: "key-123",
		UserID:         123,
		Asset:          "BTC",
		AvailableDelta: -100,
		FrozenDelta:    100,
		AvailableAfter: 900,
		FrozenAfter:    100,
		Reason:         ReasonOrderFreeze,
		RefType:        "ORDER",
		RefID:          "456",
		Note:           "freeze for order",
		CreatedAt:      1000,
	}

	if entry.LedgerID != 1 {
		t.Fatalf("expected LedgerID=1, got %d", entry.LedgerID)
	}
	if entry.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", entry.UserID)
	}
	if entry.Asset != "BTC" {
		t.Fatalf("expected Asset=BTC, got %s", entry.Asset)
	}
	if entry.AvailableDelta != -100 {
		t.Fatalf("expected AvailableDelta=-100, got %d", entry.AvailableDelta)
	}
	if entry.Reason != ReasonOrderFreeze {
		t.Fatalf("expected Reason=ReasonOrderFreeze, got %d", entry.Reason)
	}
}

func TestNewBalanceRepository(t *testing.T) {
	repo := NewBalanceRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestErrInsufficientBalance(t *testing.T) {
	if ErrInsufficientBalance == nil {
		t.Fatal("ErrInsufficientBalance should not be nil")
	}
	if ErrInsufficientBalance.Error() != "insufficient balance" {
		t.Fatalf("expected 'insufficient balance', got %s", ErrInsufficientBalance.Error())
	}
}

func TestErrIdempotencyConflict(t *testing.T) {
	if ErrIdempotencyConflict == nil {
		t.Fatal("ErrIdempotencyConflict should not be nil")
	}
	if ErrIdempotencyConflict.Error() != "idempotency conflict" {
		t.Fatalf("expected 'idempotency conflict', got %s", ErrIdempotencyConflict.Error())
	}
}

func TestLedgerReasonConstants(t *testing.T) {
	if ReasonOrderFreeze != 1 {
		t.Fatalf("expected ReasonOrderFreeze=1, got %d", ReasonOrderFreeze)
	}
	if ReasonOrderUnfreeze != 2 {
		t.Fatalf("expected ReasonOrderUnfreeze=2, got %d", ReasonOrderUnfreeze)
	}
	if ReasonTradeSettle != 3 {
		t.Fatalf("expected ReasonTradeSettle=3, got %d", ReasonTradeSettle)
	}
	if ReasonFee != 4 {
		t.Fatalf("expected ReasonFee=4, got %d", ReasonFee)
	}
	if ReasonDeposit != 5 {
		t.Fatalf("expected ReasonDeposit=5, got %d", ReasonDeposit)
	}
	if ReasonWithdraw != 6 {
		t.Fatalf("expected ReasonWithdraw=6, got %d", ReasonWithdraw)
	}
}

func TestBalanceTotal(t *testing.T) {
	balance := &Balance{
		Available: 1000,
		Frozen:    200,
	}

	total := balance.Available + balance.Frozen
	if total != 1200 {
		t.Fatalf("expected total=1200, got %d", total)
	}
}

func TestErrNotFound(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound should not be nil")
	}
	if ErrNotFound.Error() != "not found" {
		t.Fatalf("expected 'not found', got %s", ErrNotFound.Error())
	}
}
