package service

import (
	"context"
	"errors"
	"testing"

	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/repository"
)

type idempotentFreezeClearingClient struct {
	freezeCalls int
	seenFreeze  map[string]struct{}
	frozenByUID map[int64]int64
}

func newIdempotentFreezeClearingClient() *idempotentFreezeClearingClient {
	return &idempotentFreezeClearingClient{
		seenFreeze:  make(map[string]struct{}),
		frozenByUID: make(map[int64]int64),
	}
}

func (m *idempotentFreezeClearingClient) Freeze(_ context.Context, req *client.FreezeRequest) error {
	m.freezeCalls++
	if _, ok := m.seenFreeze[req.IdempotencyKey]; ok {
		return nil
	}
	m.seenFreeze[req.IdempotencyKey] = struct{}{}
	m.frozenByUID[req.UserID] += req.Amount
	return nil
}

func (m *idempotentFreezeClearingClient) Unfreeze(_ context.Context, req *client.UnfreezeRequest) error {
	m.frozenByUID[req.UserID] -= req.Amount
	return nil
}

func (m *idempotentFreezeClearingClient) Deduct(_ context.Context, _ *client.DeductRequest) error {
	return nil
}

func (m *idempotentFreezeClearingClient) Credit(_ context.Context, _ *client.CreditRequest) error {
	return nil
}

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

func TestWalletService_RequestWithdraw_DoesNotUnfreezeOnCreateFailure(t *testing.T) {
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
	if len(clearing.unfreezeCalls) != 0 {
		t.Fatalf("did not expect auto unfreeze on create failure")
	}
}

func TestWalletService_RequestWithdraw_RetrySameIdempotencyAfterCreateFailureKeepsFundsFrozen(t *testing.T) {
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

	clearing := newIdempotentFreezeClearingClient()
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	req := &WithdrawRequest{
		IdempotencyKey: "k1",
		UserID:         1,
		Asset:          "USDT",
		Network:        "TRON",
		Amount:         10,
		Address:        "Txxx",
	}

	if _, err := svc.RequestWithdraw(context.Background(), req); err == nil {
		t.Fatalf("expected first attempt error")
	}
	if got := clearing.frozenByUID[1]; got != 10 {
		t.Fatalf("expected funds to stay frozen after create failure, got %d", got)
	}

	repo.createWithdrawalErr = nil
	resp, err := svc.RequestWithdraw(context.Background(), req)
	if err != nil {
		t.Fatalf("retry request withdraw: %v", err)
	}
	if resp == nil || resp.Withdrawal == nil {
		t.Fatalf("expected withdrawal on retry")
	}
	if got := clearing.frozenByUID[1]; got != 10 {
		t.Fatalf("expected frozen amount to remain 10 on same-key retry, got %d", got)
	}
	if clearing.freezeCalls != 2 {
		t.Fatalf("expected two freeze attempts (second idempotent), got %d", clearing.freezeCalls)
	}
}

func TestWalletService_RequestWithdraw_CreateFailureButRecordExistsReturnsIdempotentSuccess(t *testing.T) {
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
	repo.createWithdrawalPersistBeforeErr = true
	repo.createWithdrawalErr = errors.New("duplicate key")

	clearing := newMockClearingClient()
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	resp, err := svc.RequestWithdraw(context.Background(), &WithdrawRequest{
		IdempotencyKey: "k1",
		UserID:         1,
		Asset:          "USDT",
		Network:        "TRON",
		Amount:         10,
		Address:        "Txxx",
	})
	if err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
	if resp == nil || resp.Withdrawal == nil {
		t.Fatalf("expected withdrawal in response")
	}
	if resp.Withdrawal.IdempotencyKey != "k1" {
		t.Fatalf("unexpected idempotency key: %s", resp.Withdrawal.IdempotencyKey)
	}
	if len(clearing.freezeCalls) != 1 {
		t.Fatalf("expected one freeze call, got %d", len(clearing.freezeCalls))
	}
	if len(clearing.unfreezeCalls) != 0 {
		t.Fatalf("did not expect unfreeze call")
	}
}

func TestWalletService_RejectWithdraw_ClaimsStatusBeforeUnfreeze(t *testing.T) {
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
	if got := repo.withdrawals[1].Status; got != repository.WithdrawStatusRejected {
		t.Fatalf("expected status rejected, got %d", got)
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

func TestWalletService_ApproveWithdraw_InvalidState(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusRejected,
	}

	svc := NewWalletService(repo, &mockIDGen{}, newMockClearingClient(), nil)
	err := svc.ApproveWithdraw(context.Background(), 1, 100)
	if err == nil {
		t.Fatalf("expected invalid state error")
	}
}

func TestWalletService_CompleteWithdraw_RequiresApproved(t *testing.T) {
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
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)
	err := svc.CompleteWithdraw(context.Background(), 1, "txid")
	if err == nil {
		t.Fatalf("expected invalid state error")
	}
	if len(clearing.deductCalls) != 0 {
		t.Fatalf("did not expect deduct call")
	}
}

func TestWalletService_RequestWithdraw_Validation(t *testing.T) {
	repo := newMockWalletRepository()
	svc := NewWalletService(repo, &mockIDGen{}, newMockClearingClient(), nil)

	resp, err := svc.RequestWithdraw(context.Background(), &WithdrawRequest{
		IdempotencyKey: "",
		UserID:         1,
		Asset:          "USDT",
		Network:        "TRON",
		Amount:         10,
		Address:        "Txxx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.ErrorCode != "INVALID_PARAM" {
		t.Fatalf("expected INVALID_PARAM response, got %+v", resp)
	}
}

func TestWalletService_ApproveWithdraw_CASConflictButAlreadyApproved(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusPending,
	}
	repo.forceCASNoUpdate = true
	repo.casNoUpdateStatus = repository.WithdrawStatusApproved

	svc := NewWalletService(repo, &mockIDGen{}, newMockClearingClient(), nil)
	if err := svc.ApproveWithdraw(context.Background(), 1, 100); err != nil {
		t.Fatalf("expected idempotent success on CAS conflict, got %v", err)
	}
}

func TestWalletService_RejectWithdraw_CASConflictSkipsUnfreeze(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusPending,
	}
	repo.forceCASNoUpdate = true
	repo.casNoUpdateStatus = repository.WithdrawStatusApproved

	clearing := newMockClearingClient()
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	err := svc.RejectWithdraw(context.Background(), 1, 100)
	if err == nil {
		t.Fatalf("expected invalid state error")
	}
	if !errors.Is(err, ErrInvalidWithdrawState) {
		t.Fatalf("expected ErrInvalidWithdrawState, got %v", err)
	}
	if len(clearing.unfreezeCalls) != 0 {
		t.Fatalf("unexpected unfreeze call on CAS conflict")
	}
}

func TestWalletService_RejectWithdraw_AlreadyRejectedStillUnfreezeIdempotently(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusRejected,
	}

	clearing := newMockClearingClient()
	svc := NewWalletService(repo, &mockIDGen{}, clearing, nil)

	if err := svc.RejectWithdraw(context.Background(), 1, 100); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if len(clearing.unfreezeCalls) != 1 {
		t.Fatalf("expected unfreeze call for idempotent recovery, got %d", len(clearing.unfreezeCalls))
	}
}

func TestWalletService_CompleteWithdraw_CASConflictReturnsInvalidState(t *testing.T) {
	repo := newMockWalletRepository()
	repo.withdrawals[1] = &repository.Withdrawal{
		WithdrawID: 1,
		UserID:     1,
		Asset:      "USDT",
		Network:    "TRON",
		Amount:     10,
		Status:     repository.WithdrawStatusApproved,
	}
	repo.forceCASNoUpdate = true
	repo.casNoUpdateStatus = repository.WithdrawStatusRejected

	svc := NewWalletService(repo, &mockIDGen{}, newMockClearingClient(), nil)
	err := svc.CompleteWithdraw(context.Background(), 1, "txid")
	if err == nil {
		t.Fatalf("expected invalid state error")
	}
	if !errors.Is(err, ErrInvalidWithdrawState) {
		t.Fatalf("expected ErrInvalidWithdrawState, got %v", err)
	}
}
