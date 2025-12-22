package repository

import (
	"testing"
)

func TestAssetStruct(t *testing.T) {
	asset := &Asset{
		Asset:       "BTC",
		Name:        "Bitcoin",
		Precision:   8,
		Status:      1,
		CreatedAtMs: 1000,
		UpdatedAtMs: 2000,
	}

	if asset.Asset != "BTC" {
		t.Fatalf("expected Asset=BTC, got %s", asset.Asset)
	}
	if asset.Name != "Bitcoin" {
		t.Fatalf("expected Name=Bitcoin, got %s", asset.Name)
	}
	if asset.Precision != 8 {
		t.Fatalf("expected Precision=8, got %d", asset.Precision)
	}
}

func TestNetworkStruct(t *testing.T) {
	network := &Network{
		Asset:                  "USDT",
		Network:                "TRC20",
		DepositEnabled:         true,
		WithdrawEnabled:        true,
		MinWithdraw:            10.0,
		WithdrawFee:            1.0,
		ConfirmationsRequired:  20,
		ContractAddress:        "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		Status:                 1,
	}

	if network.Asset != "USDT" {
		t.Fatalf("expected Asset=USDT, got %s", network.Asset)
	}
	if network.Network != "TRC20" {
		t.Fatalf("expected Network=TRC20, got %s", network.Network)
	}
	if !network.DepositEnabled {
		t.Fatal("expected DepositEnabled=true")
	}
	if !network.WithdrawEnabled {
		t.Fatal("expected WithdrawEnabled=true")
	}
	if network.MinWithdraw != 10.0 {
		t.Fatalf("expected MinWithdraw=10.0, got %f", network.MinWithdraw)
	}
}

func TestDepositAddressStruct(t *testing.T) {
	addr := &DepositAddress{
		UserID:      123,
		Asset:       "BTC",
		Network:     "BTC",
		Address:     "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
		Tag:         "",
		CreatedAtMs: 1000,
	}

	if addr.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", addr.UserID)
	}
	if addr.Asset != "BTC" {
		t.Fatalf("expected Asset=BTC, got %s", addr.Asset)
	}
	if addr.Address != "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2" {
		t.Fatalf("expected Address=1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2, got %s", addr.Address)
	}
}

func TestDepositStruct(t *testing.T) {
	deposit := &Deposit{
		DepositID:     1,
		UserID:        123,
		Asset:         "BTC",
		Network:       "BTC",
		Amount:        1.5,
		Txid:          "abc123",
		Vout:          0,
		Confirmations: 6,
		Status:        DepositStatusCredited,
		CreditedAtMs:  2000,
		CreatedAtMs:   1000,
		UpdatedAtMs:   2000,
	}

	if deposit.DepositID != 1 {
		t.Fatalf("expected DepositID=1, got %d", deposit.DepositID)
	}
	if deposit.Amount != 1.5 {
		t.Fatalf("expected Amount=1.5, got %f", deposit.Amount)
	}
	if deposit.Status != DepositStatusCredited {
		t.Fatalf("expected Status=DepositStatusCredited, got %d", deposit.Status)
	}
}

func TestWithdrawalStruct(t *testing.T) {
	withdrawal := &Withdrawal{
		WithdrawID:     1,
		IdempotencyKey: "key-1",
		UserID:         123,
		Asset:          "BTC",
		Network:        "BTC",
		Amount:         1.0,
		Fee:            0.0001,
		Address:        "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
		Tag:            "",
		Status:         WithdrawStatusPending,
		Txid:           "",
		RequestedAtMs:  1000,
	}

	if withdrawal.WithdrawID != 1 {
		t.Fatalf("expected WithdrawID=1, got %d", withdrawal.WithdrawID)
	}
	if withdrawal.Amount != 1.0 {
		t.Fatalf("expected Amount=1.0, got %f", withdrawal.Amount)
	}
	if withdrawal.Status != WithdrawStatusPending {
		t.Fatalf("expected Status=WithdrawStatusPending, got %d", withdrawal.Status)
	}
}

func TestDepositStatusConstants(t *testing.T) {
	if DepositStatusPending != 1 {
		t.Fatalf("expected DepositStatusPending=1, got %d", DepositStatusPending)
	}
	if DepositStatusConfirmed != 2 {
		t.Fatalf("expected DepositStatusConfirmed=2, got %d", DepositStatusConfirmed)
	}
	if DepositStatusCredited != 3 {
		t.Fatalf("expected DepositStatusCredited=3, got %d", DepositStatusCredited)
	}
}

func TestWithdrawStatusConstants(t *testing.T) {
	if WithdrawStatusPending != 1 {
		t.Fatalf("expected WithdrawStatusPending=1, got %d", WithdrawStatusPending)
	}
	if WithdrawStatusApproved != 2 {
		t.Fatalf("expected WithdrawStatusApproved=2, got %d", WithdrawStatusApproved)
	}
	if WithdrawStatusRejected != 3 {
		t.Fatalf("expected WithdrawStatusRejected=3, got %d", WithdrawStatusRejected)
	}
	if WithdrawStatusProcessing != 4 {
		t.Fatalf("expected WithdrawStatusProcessing=4, got %d", WithdrawStatusProcessing)
	}
	if WithdrawStatusCompleted != 5 {
		t.Fatalf("expected WithdrawStatusCompleted=5, got %d", WithdrawStatusCompleted)
	}
	if WithdrawStatusFailed != 6 {
		t.Fatalf("expected WithdrawStatusFailed=6, got %d", WithdrawStatusFailed)
	}
}

func TestNewWalletRepository(t *testing.T) {
	repo := NewWalletRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
