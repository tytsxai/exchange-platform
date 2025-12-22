package service

import (
	"context"
	"errors"
	"testing"

	"github.com/exchange/wallet/internal/repository"
)

func TestWalletService_ProcessDeposit_CreditsOnConfirmations(t *testing.T) {
	repo := newMockWalletRepository()
	repo.networks = []*repository.Network{
		{
			Asset:                 "USDT",
			Network:               "TRON",
			DepositEnabled:        true,
			WithdrawEnabled:       true,
			ConfirmationsRequired: 2,
			WithdrawFee:           1,
			MinWithdraw:           1,
			Status:                1,
		},
	}

	clearing := newMockClearingClient()
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	if err := svc.ProcessDeposit(context.Background(), 1, "USDT", "TRON", "tx1", 0, 100, 2); err != nil {
		t.Fatalf("process deposit: %v", err)
	}
	if len(clearing.creditCalls) != 1 {
		t.Fatalf("expected 1 credit call, got %d", len(clearing.creditCalls))
	}
	if got := repo.deposits[1].Status; got != repository.DepositStatusCredited {
		t.Fatalf("expected deposit credited, got %d", got)
	}
}

func TestWalletService_RequestWithdraw_UnfreezesOnCreateFailure(t *testing.T) {
	repo := newMockWalletRepository()
	repo.networks = []*repository.Network{
		{
			Asset:           "USDT",
			Network:         "TRON",
			DepositEnabled:  true,
			WithdrawEnabled: true,
			MinWithdraw:     1,
			WithdrawFee:     1,
			Status:          1,
		},
	}
	repo.createWithdrawalErr = errors.New("db down")

	clearing := newMockClearingClient()
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	_, err := svc.RequestWithdraw(context.Background(), &WithdrawRequest{
		IdempotencyKey: "k1",
		UserID:         1,
		Asset:          "USDT",
		Network:        "TRON",
		Amount:         10,
		Address:        "Txxx",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(clearing.freezeCalls) != 1 {
		t.Fatalf("expected freeze called")
	}
	if len(clearing.unfreezeCalls) != 1 {
		t.Fatalf("expected unfreeze called")
	}
}

func TestWalletService_RejectWithdraw_DoesNotUpdateStatusOnUnfreezeFailure(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusPending,
	}

	clearing := newMockClearingClient()
	clearing.unfreezeErr = errors.New("rpc error")
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	err := svc.RejectWithdraw(context.Background(), 1, 100)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := repo.withdrawals[1].Status; got != repository.WithdrawStatusPending {
		t.Fatalf("expected status unchanged, got %d", got)
	}
}

func TestWalletService_CompleteWithdraw_DoesNotMarkCompletedOnDeductFailure(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusApproved,
	}

	clearing := newMockClearingClient()
	clearing.deductErr = errors.New("rpc error")
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	err := svc.CompleteWithdraw(context.Background(), 1, "txid")
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := repo.withdrawals[1].Status; got == repository.WithdrawStatusCompleted {
		t.Fatalf("did not expect completed status")
	}
}
