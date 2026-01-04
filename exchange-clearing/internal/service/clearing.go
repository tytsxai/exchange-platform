package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/exchange/clearing/internal/repository"
)

type ClearingService struct {
	db        *sql.DB
	balRepo   *repository.BalanceRepository
	idGen     IDGenerator
	publisher balancePublisher
}

type balancePublisher interface {
	PublishFrozenEvent(ctx context.Context, userID int64, asset string, amount int64) error
	PublishUnfrozenEvent(ctx context.Context, userID int64, asset string, amount int64) error
	PublishSettledEvent(ctx context.Context, userID int64, data interface{}) error
}

type IDGenerator interface {
	NextID() int64
}

func NewClearingService(db *sql.DB, idGen IDGenerator) *ClearingService {
	return &ClearingService{
		db:      db,
		balRepo: repository.NewBalanceRepository(db),
		idGen:   idGen,
	}
}

func (s *ClearingService) SetPublisher(publisher balancePublisher) {
	s.publisher = publisher
}

type FreezeRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

type FreezeResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

func (s *ClearingService) Freeze(ctx context.Context, req *FreezeRequest) (*FreezeResponse, error) {
	entry := &repository.LedgerEntry{
		LedgerID:       s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: -req.Amount,
		FrozenDelta:    req.Amount,
		Reason:         repository.ReasonOrderFreeze,
		RefType:        req.RefType,
		RefID:          req.RefID,
		CreatedAt:      time.Now().UnixMilli(),
	}

	err := s.withOptimisticRetry(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return s.balRepo.Freeze(ctx, tx, entry)
	})
	if err != nil {
		if err == repository.ErrInsufficientBalance {
			return &FreezeResponse{Success: false, ErrorCode: "INSUFFICIENT_BALANCE"}, nil
		}
		if err == repository.ErrIdempotencyConflict {
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &FreezeResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("freeze: %w", err)
	}

	if s.publisher != nil {
		if pubErr := s.publisher.PublishFrozenEvent(ctx, req.UserID, req.Asset, req.Amount); pubErr != nil {
			log.Printf("publish frozen event error: %v", pubErr)
		}
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &FreezeResponse{Success: true, Balance: balance}, nil
}

type UnfreezeRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

type UnfreezeResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

func (s *ClearingService) Unfreeze(ctx context.Context, req *UnfreezeRequest) (*UnfreezeResponse, error) {
	entry := &repository.LedgerEntry{
		LedgerID:       s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: req.Amount,
		FrozenDelta:    -req.Amount,
		Reason:         repository.ReasonOrderUnfreeze,
		RefType:        req.RefType,
		RefID:          req.RefID,
		CreatedAt:      time.Now().UnixMilli(),
	}

	err := s.withOptimisticRetry(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return s.balRepo.Unfreeze(ctx, tx, entry)
	})

	if err != nil {
		if err == repository.ErrIdempotencyConflict {
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &UnfreezeResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("unfreeze: %w", err)
	}

	if s.publisher != nil {
		if pubErr := s.publisher.PublishUnfrozenEvent(ctx, req.UserID, req.Asset, req.Amount); pubErr != nil {
			log.Printf("publish unfrozen event error: %v", pubErr)
		}
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &UnfreezeResponse{Success: true, Balance: balance}, nil
}

type DeductRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

type DeductResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

func (s *ClearingService) Deduct(ctx context.Context, req *DeductRequest) (*DeductResponse, error) {
	entry := &repository.LedgerEntry{
		LedgerID:       s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: 0,
		FrozenDelta:    -req.Amount,
		Reason:         repository.ReasonWithdraw,
		RefType:        req.RefType,
		RefID:          req.RefID,
		CreatedAt:      time.Now().UnixMilli(),
	}

	err := s.withOptimisticRetry(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return s.balRepo.Deduct(ctx, tx, entry)
	})

	if err != nil {
		if err == repository.ErrInsufficientBalance {
			return &DeductResponse{Success: false, ErrorCode: "INSUFFICIENT_BALANCE"}, nil
		}
		if err == repository.ErrIdempotencyConflict {
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &DeductResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("deduct: %w", err)
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &DeductResponse{Success: true, Balance: balance}, nil
}

type CreditRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

type CreditResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

func (s *ClearingService) Credit(ctx context.Context, req *CreditRequest) (*CreditResponse, error) {
	if req.Amount <= 0 {
		return &CreditResponse{Success: false, ErrorCode: "INVALID_AMOUNT"}, nil
	}

	entry := &repository.LedgerEntry{
		LedgerID:       s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: req.Amount,
		FrozenDelta:    0,
		Reason:         repository.ReasonDeposit,
		RefType:        req.RefType,
		RefID:          req.RefID,
		CreatedAt:      time.Now().UnixMilli(),
	}

	err := s.withOptimisticRetry(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return s.balRepo.Credit(ctx, tx, entry)
	})

	if err != nil {
		if err == repository.ErrIdempotencyConflict {
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &CreditResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("credit: %w", err)
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &CreditResponse{Success: true, Balance: balance}, nil
}

type SettleTradeRequest struct {
	IdempotencyKey string
	TradeID        string
	Symbol         string

	MakerUserID     int64
	MakerOrderID    string
	MakerBaseDelta  int64
	MakerQuoteDelta int64
	MakerFee        int64
	MakerFeeAsset   string

	TakerUserID     int64
	TakerOrderID    string
	TakerBaseDelta  int64
	TakerQuoteDelta int64
	TakerFee        int64
	TakerFeeAsset   string

	BaseAsset  string
	QuoteAsset string
}

type SettleTradeResponse struct {
	Success   bool
	ErrorCode string
}

func (s *ClearingService) SettleTrade(ctx context.Context, req *SettleTradeRequest) (*SettleTradeResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	var entries []*repository.LedgerEntry

	if req.MakerBaseDelta != 0 {
		entries = append(entries, &repository.LedgerEntry{
			LedgerID:       s.idGen.NextID(),
			IdempotencyKey: fmt.Sprintf("settle:%s:maker:base", req.TradeID),
			UserID:         req.MakerUserID,
			Asset:          req.BaseAsset,
			AvailableDelta: maxInt64(req.MakerBaseDelta, 0),
			FrozenDelta:    minInt64(req.MakerBaseDelta, 0),
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
			CreatedAt:      now,
		})
	}

	if req.MakerQuoteDelta != 0 {
		entries = append(entries, &repository.LedgerEntry{
			LedgerID:       s.idGen.NextID(),
			IdempotencyKey: fmt.Sprintf("settle:%s:maker:quote", req.TradeID),
			UserID:         req.MakerUserID,
			Asset:          req.QuoteAsset,
			AvailableDelta: maxInt64(req.MakerQuoteDelta, 0),
			FrozenDelta:    minInt64(req.MakerQuoteDelta, 0),
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
			CreatedAt:      now,
		})
	}

	if req.MakerFee > 0 {
		entries = append(entries, &repository.LedgerEntry{
			LedgerID:       s.idGen.NextID(),
			IdempotencyKey: fmt.Sprintf("settle:%s:maker:fee", req.TradeID),
			UserID:         req.MakerUserID,
			Asset:          req.MakerFeeAsset,
			AvailableDelta: -req.MakerFee,
			Reason:         repository.ReasonFee,
			RefType:        "TRADE",
			RefID:          req.TradeID,
			CreatedAt:      now,
		})
	}

	if req.TakerBaseDelta != 0 {
		entries = append(entries, &repository.LedgerEntry{
			LedgerID:       s.idGen.NextID(),
			IdempotencyKey: fmt.Sprintf("settle:%s:taker:base", req.TradeID),
			UserID:         req.TakerUserID,
			Asset:          req.BaseAsset,
			AvailableDelta: maxInt64(req.TakerBaseDelta, 0),
			FrozenDelta:    minInt64(req.TakerBaseDelta, 0),
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
			CreatedAt:      now,
		})
	}

	if req.TakerQuoteDelta != 0 {
		entries = append(entries, &repository.LedgerEntry{
			LedgerID:       s.idGen.NextID(),
			IdempotencyKey: fmt.Sprintf("settle:%s:taker:quote", req.TradeID),
			UserID:         req.TakerUserID,
			Asset:          req.QuoteAsset,
			AvailableDelta: maxInt64(req.TakerQuoteDelta, 0),
			FrozenDelta:    minInt64(req.TakerQuoteDelta, 0),
			Reason:         repository.ReasonTradeSettle,
			RefType:        "TRADE",
			RefID:          req.TradeID,
			CreatedAt:      now,
		})
	}

	if req.TakerFee > 0 {
		entries = append(entries, &repository.LedgerEntry{
			LedgerID:       s.idGen.NextID(),
			IdempotencyKey: fmt.Sprintf("settle:%s:taker:fee", req.TradeID),
			UserID:         req.TakerUserID,
			Asset:          req.TakerFeeAsset,
			AvailableDelta: -req.TakerFee,
			Reason:         repository.ReasonFee,
			RefType:        "TRADE",
			RefID:          req.TradeID,
			CreatedAt:      now,
		})
	}

	if err := s.balRepo.Settle(ctx, tx, entries); err != nil {
		return nil, fmt.Errorf("settle: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	if s.publisher != nil {
		payload := map[string]any{"tradeId": req.TradeID, "symbol": req.Symbol}
		if pubErr := s.publisher.PublishSettledEvent(ctx, req.MakerUserID, payload); pubErr != nil {
			log.Printf("publish settled maker event error: %v", pubErr)
		}
		if pubErr := s.publisher.PublishSettledEvent(ctx, req.TakerUserID, payload); pubErr != nil {
			log.Printf("publish settled taker event error: %v", pubErr)
		}
	}

	return &SettleTradeResponse{Success: true}, nil
}

func (s *ClearingService) withOptimisticRetry(ctx context.Context, op func(context.Context, *sql.Tx) error) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		err = op(ctx, tx)
		if err == nil {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit: %w", err)
			}
			return nil
		}
		rbErr := tx.Rollback()
		if rbErr != nil && rbErr != sql.ErrTxDone {
			return fmt.Errorf("rollback: %w", rbErr)
		}
		lastErr = err
		if errors.Is(err, repository.ErrIdempotencyConflict) {
			return err
		}
		if !errors.Is(err, repository.ErrOptimisticLockFailed) {
			return err
		}
	}
	return lastErr
}

func (s *ClearingService) GetBalance(ctx context.Context, userID int64, asset string) (*repository.Balance, error) {
	return s.balRepo.GetBalance(ctx, userID, asset)
}

func (s *ClearingService) GetBalances(ctx context.Context, userID int64) ([]*repository.Balance, error) {
	return s.balRepo.GetBalances(ctx, userID)
}

func (s *ClearingService) ListLedger(ctx context.Context, userID int64, asset string, limit int) ([]*repository.LedgerEntry, error) {
	return s.balRepo.ListLedger(ctx, userID, asset, limit)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
