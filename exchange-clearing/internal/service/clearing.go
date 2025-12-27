// Package service 清算服务
package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/exchange/clearing/internal/repository"
)

// ClearingService 清算服务
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

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// NewClearingService 创建清算服务
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

// FreezeRequest 冻结请求
type FreezeRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

// FreezeResponse 冻结响应
type FreezeResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

// Freeze 冻结资金（下单时调用）
func (s *ClearingService) Freeze(ctx context.Context, req *FreezeRequest) (*FreezeResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	entry := &repository.LedgerEntry{
		LedgerID:       s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: -req.Amount, // 可用减少
		FrozenDelta:    req.Amount,  // 冻结增加
		Reason:         repository.ReasonOrderFreeze,
		RefType:        req.RefType,
		RefID:          req.RefID,
		CreatedAt:      time.Now().UnixMilli(),
	}

	err = s.balRepo.Freeze(ctx, tx, entry)
	if err != nil {
		if err == repository.ErrInsufficientBalance {
			return &FreezeResponse{Success: false, ErrorCode: "INSUFFICIENT_BALANCE"}, nil
		}
		if err == repository.ErrIdempotencyConflict {
			// 幂等：返回成功
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &FreezeResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("freeze: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	if s.publisher != nil {
		if pubErr := s.publisher.PublishFrozenEvent(ctx, req.UserID, req.Asset, req.Amount); pubErr != nil {
			log.Printf("publish frozen event error: %v", pubErr)
		}
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &FreezeResponse{Success: true, Balance: balance}, nil
}

// UnfreezeRequest 解冻请求
type UnfreezeRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

// UnfreezeResponse 解冻响应
type UnfreezeResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

// Unfreeze 解冻资金（撤单时调用）
func (s *ClearingService) Unfreeze(ctx context.Context, req *UnfreezeRequest) (*UnfreezeResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	entry := &repository.LedgerEntry{
		LedgerID:       s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		AvailableDelta: req.Amount,  // 可用增加
		FrozenDelta:    -req.Amount, // 冻结减少
		Reason:         repository.ReasonOrderUnfreeze,
		RefType:        req.RefType,
		RefID:          req.RefID,
		CreatedAt:      time.Now().UnixMilli(),
	}

	err = s.balRepo.Unfreeze(ctx, tx, entry)
	if err != nil {
		if err == repository.ErrIdempotencyConflict {
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &UnfreezeResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("unfreeze: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	if s.publisher != nil {
		if pubErr := s.publisher.PublishUnfrozenEvent(ctx, req.UserID, req.Asset, req.Amount); pubErr != nil {
			log.Printf("publish unfrozen event error: %v", pubErr)
		}
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &UnfreezeResponse{Success: true, Balance: balance}, nil
}

// DeductRequest 扣除请求
type DeductRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

// DeductResponse 扣除响应
type DeductResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

// Deduct 扣除冻结资金（提现完成调用）
func (s *ClearingService) Deduct(ctx context.Context, req *DeductRequest) (*DeductResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

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

	err = s.balRepo.Deduct(ctx, tx, entry)
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

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &DeductResponse{Success: true, Balance: balance}, nil
}

// CreditRequest 入账请求（充值）
type CreditRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Amount         int64
	RefType        string
	RefID          string
}

// CreditResponse 入账响应
type CreditResponse struct {
	Success   bool
	ErrorCode string
	Balance   *repository.Balance
}

// Credit 入账（充值确认后调用）
func (s *ClearingService) Credit(ctx context.Context, req *CreditRequest) (*CreditResponse, error) {
	if req.Amount <= 0 {
		return &CreditResponse{Success: false, ErrorCode: "INVALID_AMOUNT"}, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

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

	err = s.balRepo.Credit(ctx, tx, entry)
	if err != nil {
		if err == repository.ErrIdempotencyConflict {
			balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
			return &CreditResponse{Success: true, Balance: balance}, nil
		}
		return nil, fmt.Errorf("credit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	balance, _ := s.balRepo.GetBalance(ctx, req.UserID, req.Asset)
	return &CreditResponse{Success: true, Balance: balance}, nil
}

// SettleTradeRequest 清算请求
type SettleTradeRequest struct {
	IdempotencyKey string
	TradeID        string
	Symbol         string

	// Maker 侧
	MakerUserID     int64
	MakerOrderID    string
	MakerBaseDelta  int64 // base 资产变动
	MakerQuoteDelta int64 // quote 资产变动
	MakerFee        int64
	MakerFeeAsset   string

	// Taker 侧
	TakerUserID     int64
	TakerOrderID    string
	TakerBaseDelta  int64
	TakerQuoteDelta int64
	TakerFee        int64
	TakerFeeAsset   string

	BaseAsset  string
	QuoteAsset string
}

// SettleTradeResponse 清算响应
type SettleTradeResponse struct {
	Success   bool
	ErrorCode string
}

// SettleTrade 清算成交
func (s *ClearingService) SettleTrade(ctx context.Context, req *SettleTradeRequest) (*SettleTradeResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	var entries []*repository.LedgerEntry

	// Maker base 资产变动（从冻结中扣除或增加到可用）
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

	// Maker quote 资产变动
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

	// Maker 手续费
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

	// Taker base 资产变动
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

	// Taker quote 资产变动
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

	// Taker 手续费
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

// GetBalance 获取余额
func (s *ClearingService) GetBalance(ctx context.Context, userID int64, asset string) (*repository.Balance, error) {
	return s.balRepo.GetBalance(ctx, userID, asset)
}

// GetBalances 获取所有余额
func (s *ClearingService) GetBalances(ctx context.Context, userID int64) ([]*repository.Balance, error) {
	return s.balRepo.GetBalances(ctx, userID)
}

// ListLedger 查询账本
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
