package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/exchange/clearing/internal/repository"
)

func TestClearingServiceConstants(t *testing.T) {
	svc := &ClearingService{}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestFreezeRequest_Fields(t *testing.T) {
	req := &FreezeRequest{
		IdempotencyKey: "freeze-key-1",
		UserID:         123,
		Asset:          "USDT",
		Amount:         1000,
		RefType:        "ORDER",
		RefID:          "456",
	}

	if req.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", req.UserID)
	}
	if req.Asset != "USDT" {
		t.Fatalf("expected Asset=USDT, got %s", req.Asset)
	}
	if req.Amount != 1000 {
		t.Fatalf("expected Amount=1000, got %d", req.Amount)
	}
	if req.RefID != "456" {
		t.Fatalf("expected RefID=456, got %s", req.RefID)
	}
}

func TestFreezeResponse_Fields(t *testing.T) {
	resp := &FreezeResponse{
		Success:   true,
		ErrorCode: "",
	}

	if !resp.Success {
		t.Fatal("expected Success=true")
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected empty ErrorCode, got %s", resp.ErrorCode)
	}

	// Test failure case
	resp = &FreezeResponse{
		Success:   false,
		ErrorCode: "INSUFFICIENT_BALANCE",
	}

	if resp.Success {
		t.Fatal("expected Success=false")
	}
	if resp.ErrorCode != "INSUFFICIENT_BALANCE" {
		t.Fatalf("expected ErrorCode=INSUFFICIENT_BALANCE, got %s", resp.ErrorCode)
	}
}

func TestUnfreezeRequest_Fields(t *testing.T) {
	req := &UnfreezeRequest{
		IdempotencyKey: "unfreeze-key-1",
		UserID:         123,
		Asset:          "BTC",
		Amount:         500,
		RefType:        "ORDER",
		RefID:          "789",
	}

	if req.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", req.UserID)
	}
	if req.Asset != "BTC" {
		t.Fatalf("expected Asset=BTC, got %s", req.Asset)
	}
	if req.Amount != 500 {
		t.Fatalf("expected Amount=500, got %d", req.Amount)
	}
}

func TestUnfreezeResponse_Fields(t *testing.T) {
	resp := &UnfreezeResponse{
		Success:   true,
		ErrorCode: "",
	}

	if !resp.Success {
		t.Fatal("expected Success=true")
	}
}

func TestSettleTradeRequest_Fields(t *testing.T) {
	req := &SettleTradeRequest{
		IdempotencyKey:  "settle-key-1",
		TradeID:         "trade-1",
		Symbol:          "BTCUSDT",
		MakerUserID:     100,
		TakerUserID:     200,
		MakerOrderID:    "order-1000",
		TakerOrderID:    "order-2000",
		MakerBaseDelta:  -100,
		MakerQuoteDelta: 5000000,
		TakerBaseDelta:  100,
		TakerQuoteDelta: -5000000,
		MakerFee:        10,
		TakerFee:        20,
		MakerFeeAsset:   "USDT",
		TakerFeeAsset:   "BTC",
		BaseAsset:       "BTC",
		QuoteAsset:      "USDT",
	}

	if req.TradeID != "trade-1" {
		t.Fatalf("expected TradeID=trade-1, got %s", req.TradeID)
	}
	if req.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", req.Symbol)
	}
	if req.MakerUserID != 100 {
		t.Fatalf("expected MakerUserID=100, got %d", req.MakerUserID)
	}
	if req.TakerUserID != 200 {
		t.Fatalf("expected TakerUserID=200, got %d", req.TakerUserID)
	}
	if req.MakerBaseDelta != -100 {
		t.Fatalf("expected MakerBaseDelta=-100, got %d", req.MakerBaseDelta)
	}
}

func TestSettleTradeResponse_Fields(t *testing.T) {
	resp := &SettleTradeResponse{
		Success:   true,
		ErrorCode: "",
	}

	if !resp.Success {
		t.Fatal("expected Success=true")
	}

	resp = &SettleTradeResponse{
		Success:   false,
		ErrorCode: "SETTLEMENT_FAILED",
	}

	if resp.Success {
		t.Fatal("expected Success=false")
	}
	if resp.ErrorCode != "SETTLEMENT_FAILED" {
		t.Fatalf("expected ErrorCode=SETTLEMENT_FAILED, got %s", resp.ErrorCode)
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

type fakeBalancePublisher struct {
	frozenCalls int
	userID      int64
	asset       string
	amount      int64
}

func (f *fakeBalancePublisher) PublishFrozenEvent(_ context.Context, userID int64, asset string, amount int64) error {
	f.frozenCalls++
	f.userID = userID
	f.asset = asset
	f.amount = amount
	return nil
}

func (f *fakeBalancePublisher) PublishUnfrozenEvent(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (f *fakeBalancePublisher) PublishSettledEvent(_ context.Context, _ int64, _ interface{}) error {
	return nil
}

func TestNewClearingService(t *testing.T) {
	gen := &mockIDGen{}
	svc := NewClearingService(nil, gen)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

type atomicIDGen struct {
	id int64
}

func (m *atomicIDGen) NextID() int64 {
	return atomic.AddInt64(&m.id, 1)
}

func newMockService(t *testing.T, idGen IDGenerator) (*ClearingService, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	svc := NewClearingService(db, idGen)
	return svc, mock, func() {
		_ = db.Close()
	}
}

func expectCheckIdempotency(mock sqlmock.Sqlmock, key string) {
	mock.ExpectQuery(`SELECT 1 FROM exchange_clearing\.ledger_entries WHERE idempotency_key = \$1`).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
}

func expectCheckIdempotencyMiss(mock sqlmock.Sqlmock, key string) {
	mock.ExpectQuery(`SELECT 1 FROM exchange_clearing\.ledger_entries WHERE idempotency_key = \$1`).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"1"}))
}

func expectBalanceForUpdate(mock sqlmock.Sqlmock, userID int64, asset string, available, frozen, version int64) {
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms
		FROM exchange_clearing\.account_balances
		WHERE user_id = \$1 AND asset = \$2
		FOR UPDATE`).
		WithArgs(userID, asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(userID, asset, available, frozen, version, 1000))
}

func expectBalanceForUpdateEmpty(mock sqlmock.Sqlmock, userID int64, asset string) {
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms
		FROM exchange_clearing\.account_balances
		WHERE user_id = \$1 AND asset = \$2
		FOR UPDATE`).
		WithArgs(userID, asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}))
}

func expectUpdateBalance(mock sqlmock.Sqlmock, available, frozen, userID int64, asset string, version int64, rowsAffected int64) {
	mock.ExpectExec(`UPDATE exchange_clearing\.account_balances
			SET available = \$1, frozen = \$2, version = version \+ 1, updated_at_ms = \$3
			WHERE user_id = \$4 AND asset = \$5 AND version = \$6`).
		WithArgs(available, frozen, sqlmock.AnyArg(), userID, asset, version).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}

func expectInsertBalance(mock sqlmock.Sqlmock, userID int64, asset string, available, frozen int64) {
	mock.ExpectExec(`INSERT INTO exchange_clearing\.account_balances \(user_id, asset, available, frozen, version, updated_at_ms\)
			VALUES \(\$1, \$2, \$3, \$4, 1, \$5\)`).
		WithArgs(userID, asset, available, frozen, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectInsertLedger(mock sqlmock.Sqlmock, entry *repository.LedgerEntry) {
	mock.ExpectExec(`INSERT INTO exchange_clearing\.ledger_entries`).
		WithArgs(
			sqlmock.AnyArg(),
			entry.IdempotencyKey,
			entry.UserID,
			entry.Asset,
			entry.AvailableDelta,
			entry.FrozenDelta,
			entry.AvailableAfter,
			entry.FrozenAfter,
			entry.Reason,
			entry.RefType,
			entry.RefID,
			entry.Note,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestClearingServiceFreeze_InsufficientBalance(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &FreezeRequest{
		IdempotencyKey: "freeze:insufficient",
		UserID:         1,
		Asset:          "USDT",
		Amount:         100,
		RefType:        "ORDER",
		RefID:          "o-1",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 50, 0, 1)
	mock.ExpectRollback()

	resp, err := svc.Freeze(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Success {
		t.Fatal("expected freeze failure")
	}
	if resp.ErrorCode != "INSUFFICIENT_BALANCE" {
		t.Fatalf("expected INSUFFICIENT_BALANCE, got %s", resp.ErrorCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceFreeze_Idempotent(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &FreezeRequest{
		IdempotencyKey: "freeze:idempotent",
		UserID:         2,
		Asset:          "USDT",
		Amount:         100,
		RefType:        "ORDER",
		RefID:          "o-2",
	}

	mock.ExpectBegin()
	expectCheckIdempotency(mock, req.IdempotencyKey)
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 100, 0, 1, 1000))
	mock.ExpectRollback()

	resp, err := svc.Freeze(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatal("expected idempotent freeze success")
	}
	if resp.Balance == nil || resp.Balance.Available != 100 {
		t.Fatalf("expected balance available=100, got %+v", resp.Balance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceUnfreeze_InsufficientBalance(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &UnfreezeRequest{
		IdempotencyKey: "unfreeze:insufficient",
		UserID:         3,
		Asset:          "BTC",
		Amount:         10,
		RefType:        "ORDER",
		RefID:          "o-3",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 0, 5, 1)
	mock.ExpectRollback()

	_, err := svc.Unfreeze(context.Background(), req)
	if err == nil {
		t.Fatal("expected insufficient balance error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceUnfreeze_Idempotent(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &UnfreezeRequest{
		IdempotencyKey: "unfreeze:idempotent",
		UserID:         4,
		Asset:          "BTC",
		Amount:         10,
		RefType:        "ORDER",
		RefID:          "o-4",
	}

	mock.ExpectBegin()
	expectCheckIdempotency(mock, req.IdempotencyKey)
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 20, 0, 1, 1000))
	mock.ExpectRollback()

	resp, err := svc.Unfreeze(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatal("expected idempotent unfreeze success")
	}
	if resp.Balance == nil || resp.Balance.Available != 20 {
		t.Fatalf("expected balance available=20, got %+v", resp.Balance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceUnfreeze_Success(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &UnfreezeRequest{
		IdempotencyKey: "unfreeze:success",
		UserID:         7,
		Asset:          "USDT",
		Amount:         50,
		RefType:        "ORDER",
		RefID:          "o-7",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 0, 100, 1)
	expectUpdateBalance(mock, 50, 50, req.UserID, req.Asset, 1, 1)
	expectInsertLedger(mock, &repository.LedgerEntry{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: req.Amount,
		FrozenDelta:    -req.Amount,
		AvailableAfter: 50,
		FrozenAfter:    50,
		Reason:         repository.ReasonOrderUnfreeze,
		RefType:        req.RefType,
		RefID:          req.RefID,
	})
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 50, 50, 2, 1000))

	resp, err := svc.Unfreeze(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatal("expected unfreeze success")
	}
	if resp.Balance == nil || resp.Balance.Frozen != 50 {
		t.Fatalf("unexpected balance: %+v", resp.Balance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceDeduct_Success(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &DeductRequest{
		IdempotencyKey: "deduct:success",
		UserID:         9,
		Asset:          "USDT",
		Amount:         80,
		RefType:        "WITHDRAW",
		RefID:          "w-9",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 0, 100, 1)
	expectUpdateBalance(mock, 0, 20, req.UserID, req.Asset, 1, 1)
	expectInsertLedger(mock, &repository.LedgerEntry{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: 0,
		FrozenDelta:    -req.Amount,
		AvailableAfter: 0,
		FrozenAfter:    20,
		Reason:         repository.ReasonWithdraw,
		RefType:        req.RefType,
		RefID:          req.RefID,
	})
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 0, 20, 2, 1000))

	resp, err := svc.Deduct(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatal("expected deduct success")
	}
	if resp.Balance == nil || resp.Balance.Frozen != 20 {
		t.Fatalf("unexpected balance: %+v", resp.Balance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceDeduct_InsufficientBalance(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &DeductRequest{
		IdempotencyKey: "deduct:insufficient",
		UserID:         10,
		Asset:          "BTC",
		Amount:         5,
		RefType:        "WITHDRAW",
		RefID:          "w-10",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 0, 3, 1)
	mock.ExpectRollback()

	resp, err := svc.Deduct(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Success {
		t.Fatal("expected deduct failure")
	}
	if resp.ErrorCode != "INSUFFICIENT_BALANCE" {
		t.Fatalf("expected INSUFFICIENT_BALANCE, got %s", resp.ErrorCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceCredit_InvalidAmount(t *testing.T) {
	svc, _, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	resp, err := svc.Credit(context.Background(), &CreditRequest{
		IdempotencyKey: "credit:bad",
		UserID:         1,
		Asset:          "USDT",
		Amount:         0,
		RefType:        "DEPOSIT",
		RefID:          "d-1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Success {
		t.Fatalf("expected failure")
	}
	if resp.ErrorCode != "INVALID_AMOUNT" {
		t.Fatalf("expected INVALID_AMOUNT, got %s", resp.ErrorCode)
	}
}

func TestClearingServiceCredit_Success(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &CreditRequest{
		IdempotencyKey: "credit:success",
		UserID:         11,
		Asset:          "USDT",
		Amount:         100,
		RefType:        "DEPOSIT",
		RefID:          "d-11",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 50, 0, 1)
	expectUpdateBalance(mock, 150, 0, req.UserID, req.Asset, 1, 1)
	expectInsertLedger(mock, &repository.LedgerEntry{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: req.Amount,
		FrozenDelta:    0,
		AvailableAfter: 150,
		FrozenAfter:    0,
		Reason:         repository.ReasonDeposit,
		RefType:        req.RefType,
		RefID:          req.RefID,
	})
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 150, 0, 2, 1000))

	resp, err := svc.Credit(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success")
	}
	if resp.Balance == nil || resp.Balance.Available != 150 {
		t.Fatalf("unexpected balance: %+v", resp.Balance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceCredit_Idempotent(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &CreditRequest{
		IdempotencyKey: "credit:idempotent",
		UserID:         12,
		Asset:          "USDT",
		Amount:         100,
		RefType:        "DEPOSIT",
		RefID:          "d-12",
	}

	mock.ExpectBegin()
	expectCheckIdempotency(mock, req.IdempotencyKey)
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 150, 0, 2, 1000))
	mock.ExpectRollback()

	resp, err := svc.Credit(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success")
	}
	if resp.Balance == nil || resp.Balance.Available != 150 {
		t.Fatalf("unexpected balance: %+v", resp.Balance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceFreeze_OptimisticLockConflict(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &FreezeRequest{
		IdempotencyKey: "freeze:optimistic",
		UserID:         5,
		Asset:          "USDT",
		Amount:         50,
		RefType:        "ORDER",
		RefID:          "o-5",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 100, 0, 1)
	expectUpdateBalance(mock, 50, 50, req.UserID, req.Asset, 1, 0)
	mock.ExpectRollback()

	_, err := svc.Freeze(context.Background(), req)
	if err == nil {
		t.Fatal("expected optimistic lock error")
	}
	if !strings.Contains(err.Error(), "optimistic lock failed") {
		t.Fatalf("expected optimistic lock error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceFreeze_LedgerIntegrity(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()
	pub := &fakeBalancePublisher{}
	svc.SetPublisher(pub)

	req := &FreezeRequest{
		IdempotencyKey: "freeze:ledger",
		UserID:         6,
		Asset:          "USDT",
		Amount:         100,
		RefType:        "ORDER",
		RefID:          "o-6",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
	expectBalanceForUpdate(mock, req.UserID, req.Asset, 1000, 0, 1)
	expectUpdateBalance(mock, 900, 100, req.UserID, req.Asset, 1, 1)
	expectInsertLedger(mock, &repository.LedgerEntry{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: -req.Amount,
		FrozenDelta:    req.Amount,
		AvailableAfter: 900,
		FrozenAfter:    100,
		Reason:         repository.ReasonOrderFreeze,
		RefType:        req.RefType,
		RefID:          req.RefID,
	})
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(req.UserID, req.Asset).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(req.UserID, req.Asset, 900, 100, 2, 1000))

	resp, err := svc.Freeze(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatal("expected freeze success")
	}
	if resp.Balance == nil || resp.Balance.Available != 900 || resp.Balance.Frozen != 100 {
		t.Fatalf("unexpected balance: %+v", resp.Balance)
	}
	if pub.frozenCalls != 1 || pub.userID != req.UserID || pub.asset != req.Asset || pub.amount != req.Amount {
		t.Fatalf("unexpected publish frozen calls=%d user=%d asset=%s amount=%d", pub.frozenCalls, pub.userID, pub.asset, pub.amount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceSettleTrade_MakerTaker(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &SettleTradeRequest{
		IdempotencyKey:  "settle:trade",
		TradeID:         "trade-1",
		Symbol:          "BTCUSDT",
		MakerUserID:     10,
		MakerOrderID:    "m-1",
		MakerBaseDelta:  -100,
		MakerQuoteDelta: 5000,
		MakerFee:        10,
		MakerFeeAsset:   "FEE1",
		TakerUserID:     20,
		TakerOrderID:    "t-1",
		TakerBaseDelta:  100,
		TakerQuoteDelta: -5000,
		TakerFee:        1,
		TakerFeeAsset:   "FEE2",
		BaseAsset:       "BTC",
		QuoteAsset:      "USDT",
	}

	mock.ExpectBegin()

	entries := []*repository.LedgerEntry{
		{
			IdempotencyKey: "settle:trade-1:maker:base",
			UserID:         req.MakerUserID,
			Asset:          req.BaseAsset,
			AvailableDelta: 0,
			FrozenDelta:    -100,
			AvailableAfter: 0,
			FrozenAfter:    0,
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
		},
		{
			IdempotencyKey: "settle:trade-1:maker:quote",
			UserID:         req.MakerUserID,
			Asset:          req.QuoteAsset,
			AvailableDelta: 5000,
			FrozenDelta:    0,
			AvailableAfter: 5100,
			FrozenAfter:    0,
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
		},
		{
			IdempotencyKey: "settle:trade-1:maker:fee",
			UserID:         req.MakerUserID,
			Asset:          req.MakerFeeAsset,
			AvailableDelta: -10,
			FrozenDelta:    0,
			AvailableAfter: 0,
			FrozenAfter:    0,
			Reason:         repository.ReasonFee,
			RefType:        "TRADE",
			RefID:          req.TradeID,
		},
		{
			IdempotencyKey: "settle:trade-1:taker:base",
			UserID:         req.TakerUserID,
			Asset:          req.BaseAsset,
			AvailableDelta: 100,
			FrozenDelta:    0,
			AvailableAfter: 100,
			FrozenAfter:    0,
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
		},
		{
			IdempotencyKey: "settle:trade-1:taker:quote",
			UserID:         req.TakerUserID,
			Asset:          req.QuoteAsset,
			AvailableDelta: 0,
			FrozenDelta:    -5000,
			AvailableAfter: 0,
			FrozenAfter:    0,
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
		},
		{
			IdempotencyKey: "settle:trade-1:taker:fee",
			UserID:         req.TakerUserID,
			Asset:          req.TakerFeeAsset,
			AvailableDelta: -1,
			FrozenDelta:    0,
			AvailableAfter: 0,
			FrozenAfter:    0,
			Reason:         repository.ReasonFee,
			RefType:        "TRADE",
			RefID:          req.TradeID,
		},
	}

	for _, entry := range entries {
		expectCheckIdempotencyMiss(mock, entry.IdempotencyKey)
		switch entry.IdempotencyKey {
		case "settle:trade-1:maker:base":
			expectBalanceForUpdate(mock, req.MakerUserID, req.BaseAsset, 0, 100, 1)
		case "settle:trade-1:maker:quote":
			expectBalanceForUpdate(mock, req.MakerUserID, req.QuoteAsset, 100, 0, 1)
		case "settle:trade-1:maker:fee":
			expectBalanceForUpdate(mock, req.MakerUserID, req.MakerFeeAsset, 10, 0, 1)
		case "settle:trade-1:taker:base":
			expectBalanceForUpdate(mock, req.TakerUserID, req.BaseAsset, 0, 0, 1)
		case "settle:trade-1:taker:quote":
			expectBalanceForUpdate(mock, req.TakerUserID, req.QuoteAsset, 0, 5000, 1)
		case "settle:trade-1:taker:fee":
			expectBalanceForUpdate(mock, req.TakerUserID, req.TakerFeeAsset, 1, 0, 1)
		}
		expectUpdateBalance(mock, entry.AvailableAfter, entry.FrozenAfter, entry.UserID, entry.Asset, 1, 1)
		expectInsertLedger(mock, entry)
	}

	mock.ExpectCommit()

	resp, err := svc.SettleTrade(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !resp.Success {
		t.Fatal("expected settle success")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceSettleTrade_InsufficientBalance(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	req := &SettleTradeRequest{
		IdempotencyKey:  "settle:insufficient",
		TradeID:         "trade-2",
		Symbol:          "BTCUSDT",
		MakerUserID:     30,
		MakerOrderID:    "m-2",
		MakerBaseDelta:  -100,
		MakerQuoteDelta: 0,
		TakerUserID:     40,
		TakerOrderID:    "t-2",
		TakerBaseDelta:  0,
		TakerQuoteDelta: 0,
		BaseAsset:       "BTC",
		QuoteAsset:      "USDT",
	}

	mock.ExpectBegin()
	expectCheckIdempotencyMiss(mock, "settle:trade-2:maker:base")
	expectBalanceForUpdate(mock, req.MakerUserID, req.BaseAsset, 0, 50, 1)
	mock.ExpectRollback()

	_, err := svc.SettleTrade(context.Background(), req)
	if err == nil {
		t.Fatal("expected settle insufficient balance error")
	}
	if !strings.Contains(err.Error(), "insufficient balance") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceLedgerQueries(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &mockIDGen{})
	defer closeFn()

	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
		WithArgs(int64(1), "USDT").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(1, "USDT", 100, 0, 1, 1000))

	balance, err := svc.GetBalance(context.Background(), 1, "USDT")
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance.Available != 100 {
		t.Fatalf("expected available=100, got %d", balance.Available)
	}

	mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
			AddRow(1, "USDT", 100, 0, 1, 1000).
			AddRow(1, "BTC", 1, 0, 1, 1000))

	balances, err := svc.GetBalances(context.Background(), 1)
	if err != nil {
		t.Fatalf("get balances: %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(balances))
	}

	mock.ExpectQuery(`SELECT ledger_id, idempotency_key, user_id, asset, available_delta, frozen_delta,\s+available_after, frozen_after, reason, ref_type, ref_id, note, created_at_ms\s+FROM exchange_clearing\.ledger_entries\s+WHERE user_id = \$1 AND \(\$2 = '' OR asset = \$2\)\s+ORDER BY created_at_ms DESC\s+LIMIT \$3`).
		WithArgs(int64(1), "USDT", 10).
		WillReturnRows(sqlmock.NewRows([]string{"ledger_id", "idempotency_key", "user_id", "asset", "available_delta", "frozen_delta", "available_after", "frozen_after", "reason", "ref_type", "ref_id", "note", "created_at_ms"}).
			AddRow(1, "k1", 1, "USDT", -10, 10, 90, 10, repository.ReasonOrderFreeze, "ORDER", "o-1", "", 1000))

	entries, err := svc.ListLedger(context.Background(), 1, "USDT", 10)
	if err != nil {
		t.Fatalf("list ledger: %v", err)
	}
	if len(entries) != 1 || entries[0].IdempotencyKey != "k1" {
		t.Fatalf("unexpected ledger entries: %+v", entries)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClearingServiceConcurrentFreeze(t *testing.T) {
	svc, mock, closeFn := newMockService(t, &atomicIDGen{})
	defer closeFn()

	mock.MatchExpectationsInOrder(false)
	reqs := []*FreezeRequest{
		{
			IdempotencyKey: "freeze:concurrent:1",
			UserID:         11,
			Asset:          "USDT",
			Amount:         10,
			RefType:        "ORDER",
			RefID:          "o-11",
		},
		{
			IdempotencyKey: "freeze:concurrent:2",
			UserID:         12,
			Asset:          "USDT",
			Amount:         20,
			RefType:        "ORDER",
			RefID:          "o-12",
		},
	}

	for _, req := range reqs {
		mock.ExpectBegin()
		expectCheckIdempotencyMiss(mock, req.IdempotencyKey)
		expectBalanceForUpdate(mock, req.UserID, req.Asset, 100, 0, 1)
		expectUpdateBalance(mock, 100-req.Amount, req.Amount, req.UserID, req.Asset, 1, 1)
		expectInsertLedger(mock, &repository.LedgerEntry{
			IdempotencyKey: req.IdempotencyKey,
			UserID:         req.UserID,
			Asset:          req.Asset,
			AvailableDelta: -req.Amount,
			FrozenDelta:    req.Amount,
			AvailableAfter: 100 - req.Amount,
			FrozenAfter:    req.Amount,
			Reason:         repository.ReasonOrderFreeze,
			RefType:        req.RefType,
			RefID:          req.RefID,
		})
		mock.ExpectCommit()
		mock.ExpectQuery(`SELECT user_id, asset, available, frozen, version, updated_at_ms\s+FROM exchange_clearing\.account_balances\s+WHERE user_id = \$1 AND asset = \$2`).
			WithArgs(req.UserID, req.Asset).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "available", "frozen", "version", "updated_at_ms"}).
				AddRow(req.UserID, req.Asset, 100-req.Amount, req.Amount, 2, 1000))
	}

	var wg sync.WaitGroup
	errs := make([]error, len(reqs))
	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r *FreezeRequest) {
			defer wg.Done()
			resp, err := svc.Freeze(context.Background(), r)
			if err == nil && !resp.Success {
				err = errors.New("freeze failed")
			}
			errs[idx] = err
		}(i, req)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("freeze %d failed: %v", i, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestMaxInt64(t *testing.T) {
	if maxInt64(10, 20) != 20 {
		t.Fatal("expected maxInt64(10, 20) = 20")
	}
	if maxInt64(20, 10) != 20 {
		t.Fatal("expected maxInt64(20, 10) = 20")
	}
	if maxInt64(-10, 0) != 0 {
		t.Fatal("expected maxInt64(-10, 0) = 0")
	}
}

func TestMinInt64(t *testing.T) {
	if minInt64(10, 20) != 10 {
		t.Fatal("expected minInt64(10, 20) = 10")
	}
	if minInt64(20, 10) != 10 {
		t.Fatal("expected minInt64(20, 10) = 10")
	}
	if minInt64(-10, 0) != -10 {
		t.Fatal("expected minInt64(-10, 0) = -10")
	}
}
