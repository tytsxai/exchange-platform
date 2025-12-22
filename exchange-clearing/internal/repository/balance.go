// Package repository 数据访问层
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrNotFound            = errors.New("not found")
)

// Balance 账户余额
type Balance struct {
	UserID    int64
	Asset     string
	Available int64 // 最小单位整数
	Frozen    int64
	Version   int64
	UpdatedAt int64
}

// LedgerEntry 账本流水
type LedgerEntry struct {
	LedgerID       int64
	IdempotencyKey string
	UserID         int64
	Asset          string
	AvailableDelta int64
	FrozenDelta    int64
	AvailableAfter int64
	FrozenAfter    int64
	Reason         int // 1=ORDER_FREEZE, 2=ORDER_UNFREEZE, 3=TRADE_SETTLE, 4=FEE...
	RefType        string
	RefID          string
	Note           string
	CreatedAt      int64
}

// LedgerReason 账本变动原因
const (
	ReasonOrderFreeze    = 1
	ReasonOrderUnfreeze  = 2
	ReasonTradeSettle    = 3
	ReasonFee            = 4
	ReasonDeposit        = 5
	ReasonWithdraw       = 6
	ReasonWithdrawFreeze = 7
	ReasonAdjust         = 9
)

// BalanceRepository 余额仓储
type BalanceRepository struct {
	db *sql.DB
}

// NewBalanceRepository 创建仓储
func NewBalanceRepository(db *sql.DB) *BalanceRepository {
	return &BalanceRepository{db: db}
}

// GetBalance 获取余额
func (r *BalanceRepository) GetBalance(ctx context.Context, userID int64, asset string) (*Balance, error) {
	query := `
		SELECT user_id, asset, available, frozen, version, updated_at_ms
		FROM exchange_clearing.account_balances
		WHERE user_id = $1 AND asset = $2
	`
	var b Balance
	err := r.db.QueryRowContext(ctx, query, userID, asset).Scan(
		&b.UserID, &b.Asset, &b.Available, &b.Frozen, &b.Version, &b.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// 返回零余额
		return &Balance{
			UserID:    userID,
			Asset:     asset,
			Available: 0,
			Frozen:    0,
			Version:   0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query balance: %w", err)
	}
	return &b, nil
}

// GetBalances 获取用户所有余额
func (r *BalanceRepository) GetBalances(ctx context.Context, userID int64) ([]*Balance, error) {
	query := `
		SELECT user_id, asset, available, frozen, version, updated_at_ms
		FROM exchange_clearing.account_balances
		WHERE user_id = $1
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query balances: %w", err)
	}
	defer rows.Close()

	var balances []*Balance
	for rows.Next() {
		var b Balance
		if err := rows.Scan(&b.UserID, &b.Asset, &b.Available, &b.Frozen, &b.Version, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan balance: %w", err)
		}
		balances = append(balances, &b)
	}
	return balances, nil
}

// Freeze 冻结资金（下单）
func (r *BalanceRepository) Freeze(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) error {
	// 1. 检查幂等
	exists, err := r.checkIdempotency(ctx, tx, entry.IdempotencyKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrIdempotencyConflict
	}

	// 2. 获取当前余额（加锁）
	balance, err := r.getBalanceForUpdate(ctx, tx, entry.UserID, entry.Asset)
	if err != nil {
		return err
	}

	// 3. 检查余额
	amount := -entry.AvailableDelta // 冻结时 AvailableDelta 为负
	if balance.Available < amount {
		return ErrInsufficientBalance
	}

	// 4. 更新余额
	newAvailable := balance.Available + entry.AvailableDelta
	newFrozen := balance.Frozen + entry.FrozenDelta
	entry.AvailableAfter = newAvailable
	entry.FrozenAfter = newFrozen

	if err := r.updateBalance(ctx, tx, entry.UserID, entry.Asset, newAvailable, newFrozen, balance.Version); err != nil {
		return err
	}

	// 5. 写入账本
	return r.insertLedger(ctx, tx, entry)
}

// Unfreeze 解冻资金（撤单）
func (r *BalanceRepository) Unfreeze(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) error {
	// 1. 检查幂等
	exists, err := r.checkIdempotency(ctx, tx, entry.IdempotencyKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrIdempotencyConflict
	}

	// 2. 获取当前余额（加锁）
	balance, err := r.getBalanceForUpdate(ctx, tx, entry.UserID, entry.Asset)
	if err != nil {
		return err
	}

	// 3. 检查冻结余额
	amount := -entry.FrozenDelta // 解冻时 FrozenDelta 为负
	if balance.Frozen < amount {
		return ErrInsufficientBalance
	}

	// 4. 更新余额
	newAvailable := balance.Available + entry.AvailableDelta
	newFrozen := balance.Frozen + entry.FrozenDelta
	entry.AvailableAfter = newAvailable
	entry.FrozenAfter = newFrozen

	if err := r.updateBalance(ctx, tx, entry.UserID, entry.Asset, newAvailable, newFrozen, balance.Version); err != nil {
		return err
	}

	// 5. 写入账本
	return r.insertLedger(ctx, tx, entry)
}

// Deduct 扣除冻结资金（提现完成）
func (r *BalanceRepository) Deduct(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) error {
	// 1. 检查幂等
	exists, err := r.checkIdempotency(ctx, tx, entry.IdempotencyKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrIdempotencyConflict
	}

	// 2. 获取当前余额（加锁）
	balance, err := r.getBalanceForUpdate(ctx, tx, entry.UserID, entry.Asset)
	if err != nil {
		return err
	}

	// 3. 检查冻结余额
	amount := -entry.FrozenDelta
	if balance.Frozen < amount {
		return ErrInsufficientBalance
	}

	// 4. 更新余额（仅减少冻结）
	newAvailable := balance.Available + entry.AvailableDelta
	newFrozen := balance.Frozen + entry.FrozenDelta

	if newAvailable < 0 || newFrozen < 0 {
		return ErrInsufficientBalance
	}

	entry.AvailableAfter = newAvailable
	entry.FrozenAfter = newFrozen

	if err := r.updateBalance(ctx, tx, entry.UserID, entry.Asset, newAvailable, newFrozen, balance.Version); err != nil {
		return err
	}

	// 5. 写入账本
	return r.insertLedger(ctx, tx, entry)
}

// Credit 入账（充值）
func (r *BalanceRepository) Credit(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) error {
	// 1. 检查幂等
	exists, err := r.checkIdempotency(ctx, tx, entry.IdempotencyKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrIdempotencyConflict
	}

	// 2. 获取当前余额（加锁）
	balance, err := r.getBalanceForUpdate(ctx, tx, entry.UserID, entry.Asset)
	if err != nil {
		return err
	}

	// 3. 更新余额（可用增加）
	newAvailable := balance.Available + entry.AvailableDelta
	newFrozen := balance.Frozen + entry.FrozenDelta
	if newAvailable < 0 || newFrozen < 0 {
		return ErrInsufficientBalance
	}

	entry.AvailableAfter = newAvailable
	entry.FrozenAfter = newFrozen

	if err := r.updateBalance(ctx, tx, entry.UserID, entry.Asset, newAvailable, newFrozen, balance.Version); err != nil {
		return err
	}

	// 4. 写入账本
	return r.insertLedger(ctx, tx, entry)
}

// Settle 清算（成交）
func (r *BalanceRepository) Settle(ctx context.Context, tx *sql.Tx, entries []*LedgerEntry) error {
	for _, entry := range entries {
		// 1. 检查幂等
		exists, err := r.checkIdempotency(ctx, tx, entry.IdempotencyKey)
		if err != nil {
			return err
		}
		if exists {
			continue // 幂等：已处理过
		}

		// 2. 获取当前余额（加锁）
		balance, err := r.getBalanceForUpdate(ctx, tx, entry.UserID, entry.Asset)
		if err != nil {
			return err
		}

		// 3. 更新余额
		newAvailable := balance.Available + entry.AvailableDelta
		newFrozen := balance.Frozen + entry.FrozenDelta

		if newAvailable < 0 || newFrozen < 0 {
			return ErrInsufficientBalance
		}

		entry.AvailableAfter = newAvailable
		entry.FrozenAfter = newFrozen

		if err := r.updateBalance(ctx, tx, entry.UserID, entry.Asset, newAvailable, newFrozen, balance.Version); err != nil {
			return err
		}

		// 4. 写入账本
		if err := r.insertLedger(ctx, tx, entry); err != nil {
			return err
		}
	}
	return nil
}

func (r *BalanceRepository) checkIdempotency(ctx context.Context, tx *sql.Tx, key string) (bool, error) {
	query := `SELECT 1 FROM exchange_clearing.ledger_entries WHERE idempotency_key = $1`
	var exists int
	err := tx.QueryRowContext(ctx, query, key).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check idempotency: %w", err)
	}
	return true, nil
}

func (r *BalanceRepository) getBalanceForUpdate(ctx context.Context, tx *sql.Tx, userID int64, asset string) (*Balance, error) {
	query := `
		SELECT user_id, asset, available, frozen, version, updated_at_ms
		FROM exchange_clearing.account_balances
		WHERE user_id = $1 AND asset = $2
		FOR UPDATE
	`
	var b Balance
	err := tx.QueryRowContext(ctx, query, userID, asset).Scan(
		&b.UserID, &b.Asset, &b.Available, &b.Frozen, &b.Version, &b.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// 创建新记录
		return &Balance{
			UserID:    userID,
			Asset:     asset,
			Available: 0,
			Frozen:    0,
			Version:   0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get balance for update: %w", err)
	}
	return &b, nil
}

func (r *BalanceRepository) updateBalance(ctx context.Context, tx *sql.Tx, userID int64, asset string, available, frozen, version int64) error {
	now := currentTimeMs()

	if version == 0 {
		// INSERT
		query := `
			INSERT INTO exchange_clearing.account_balances (user_id, asset, available, frozen, version, updated_at_ms)
			VALUES ($1, $2, $3, $4, 1, $5)
		`
		_, err := tx.ExecContext(ctx, query, userID, asset, available, frozen, now)
		if err != nil {
			return fmt.Errorf("insert balance: %w", err)
		}
	} else {
		// UPDATE with optimistic lock
		query := `
			UPDATE exchange_clearing.account_balances
			SET available = $1, frozen = $2, version = version + 1, updated_at_ms = $3
			WHERE user_id = $4 AND asset = $5 AND version = $6
		`
		result, err := tx.ExecContext(ctx, query, available, frozen, now, userID, asset, version)
		if err != nil {
			return fmt.Errorf("update balance: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("get rows affected: %w", err)
		}
		if rows == 0 {
			return errors.New("optimistic lock failed")
		}
	}
	return nil
}

func (r *BalanceRepository) insertLedger(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) error {
	query := `
		INSERT INTO exchange_clearing.ledger_entries
		(ledger_id, idempotency_key, user_id, asset, available_delta, frozen_delta,
		 available_after, frozen_after, reason, ref_type, ref_id, note, created_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := tx.ExecContext(ctx, query,
		entry.LedgerID, entry.IdempotencyKey, entry.UserID, entry.Asset,
		entry.AvailableDelta, entry.FrozenDelta, entry.AvailableAfter, entry.FrozenAfter,
		entry.Reason, entry.RefType, entry.RefID, entry.Note, entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert ledger: %w", err)
	}
	return nil
}

// ListLedger 查询账本
func (r *BalanceRepository) ListLedger(ctx context.Context, userID int64, asset string, limit int) ([]*LedgerEntry, error) {
	query := `
		SELECT ledger_id, idempotency_key, user_id, asset, available_delta, frozen_delta,
		       available_after, frozen_after, reason, ref_type, ref_id, note, created_at_ms
		FROM exchange_clearing.ledger_entries
		WHERE user_id = $1 AND ($2 = '' OR asset = $2)
		ORDER BY created_at_ms DESC
		LIMIT $3
	`
	rows, err := r.db.QueryContext(ctx, query, userID, asset, limit)
	if err != nil {
		return nil, fmt.Errorf("query ledger: %w", err)
	}
	defer rows.Close()

	var entries []*LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(
			&e.LedgerID, &e.IdempotencyKey, &e.UserID, &e.Asset,
			&e.AvailableDelta, &e.FrozenDelta, &e.AvailableAfter, &e.FrozenAfter,
			&e.Reason, &e.RefType, &e.RefID, &e.Note, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ledger: %w", err)
		}
		entries = append(entries, &e)
	}
	return entries, nil
}

func currentTimeMs() int64 {
	return currentTimeNano() / 1e6
}

func currentTimeNano() int64 {
	return time.Now().UnixNano()
}
