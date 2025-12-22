package service

import (
	"testing"

	"github.com/exchange/wallet/internal/repository"
)

func TestWalletServiceConstants(t *testing.T) {
	svc := &WalletService{}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestWithdrawRequest_Fields(t *testing.T) {
	req := &WithdrawRequest{
		IdempotencyKey: "withdraw-key-1",
		UserID:         123,
		Asset:          "BTC",
		Network:        "BTC",
		Amount:         1.5,
		Address:        "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
		Tag:            "",
	}

	if req.IdempotencyKey != "withdraw-key-1" {
		t.Fatalf("expected IdempotencyKey=withdraw-key-1, got %s", req.IdempotencyKey)
	}
	if req.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", req.UserID)
	}
	if req.Asset != "BTC" {
		t.Fatalf("expected Asset=BTC, got %s", req.Asset)
	}
	if req.Network != "BTC" {
		t.Fatalf("expected Network=BTC, got %s", req.Network)
	}
	if req.Amount != 1.5 {
		t.Fatalf("expected Amount=1.5, got %f", req.Amount)
	}
	if req.Address != "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2" {
		t.Fatalf("expected Address=1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2, got %s", req.Address)
	}
}

func TestWithdrawResponse_Fields(t *testing.T) {
	resp := &WithdrawResponse{
		ErrorCode: "",
	}

	if resp.ErrorCode != "" {
		t.Fatalf("expected empty ErrorCode, got %s", resp.ErrorCode)
	}

	resp = &WithdrawResponse{
		ErrorCode: "WITHDRAW_DISABLED",
	}

	if resp.ErrorCode != "WITHDRAW_DISABLED" {
		t.Fatalf("expected ErrorCode=WITHDRAW_DISABLED, got %s", resp.ErrorCode)
	}
}

func TestGenerateAddress_Length(t *testing.T) {
	// Test that generateAddress produces a non-empty string
	addr := generateAddress()
	if len(addr) == 0 {
		t.Fatal("expected non-empty address")
	}
	// Address format: "0x" + 40 hex chars = 42 chars
	if len(addr) != 42 {
		t.Fatalf("expected address length=42, got %d", len(addr))
	}
}

func TestGenerateAddress_Uniqueness(t *testing.T) {
	addr1 := generateAddress()
	addr2 := generateAddress()

	if addr1 == addr2 {
		t.Fatal("expected unique addresses")
	}
}

func TestNewWalletService(t *testing.T) {
	svc := NewWalletService(nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestIDGeneratorInterface(t *testing.T) {
	var _ IDGenerator = &mockIDGen{}
}

type mockIDGen struct {
	id int64
}

func (m *mockIDGen) NextID() int64 {
	m.id++
	return m.id
}

func TestDepositStatusConstants(t *testing.T) {
	if repository.DepositStatusPending != 1 {
		t.Fatalf("expected DepositStatusPending=1, got %d", repository.DepositStatusPending)
	}
	if repository.DepositStatusConfirmed != 2 {
		t.Fatalf("expected DepositStatusConfirmed=2, got %d", repository.DepositStatusConfirmed)
	}
	if repository.DepositStatusCredited != 3 {
		t.Fatalf("expected DepositStatusCredited=3, got %d", repository.DepositStatusCredited)
	}
}

func TestWithdrawStatusConstants(t *testing.T) {
	if repository.WithdrawStatusPending != 1 {
		t.Fatalf("expected WithdrawStatusPending=1, got %d", repository.WithdrawStatusPending)
	}
	if repository.WithdrawStatusApproved != 2 {
		t.Fatalf("expected WithdrawStatusApproved=2, got %d", repository.WithdrawStatusApproved)
	}
	if repository.WithdrawStatusRejected != 3 {
		t.Fatalf("expected WithdrawStatusRejected=3, got %d", repository.WithdrawStatusRejected)
	}
}
