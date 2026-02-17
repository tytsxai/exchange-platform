// Package repository 钱包数据访问层
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// WalletRepository 钱包仓储
type WalletRepository struct {
	db *sql.DB
}

// NewWalletRepository 创建仓储
func NewWalletRepository(db *sql.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

// Asset 资产配置
type Asset struct {
	Asset       string `json:"asset"`
	Name        string `json:"name"`
	Precision   int    `json:"precision"`
	Status      int    `json:"status"`
	CreatedAtMs int64  `json:"createdAtMs"`
	UpdatedAtMs int64  `json:"updatedAtMs"`
}

// Network 网络配置
type Network struct {
	Asset                 string `json:"asset"`
	Network               string `json:"network"`
	DepositEnabled        bool   `json:"depositEnabled"`
	WithdrawEnabled       bool   `json:"withdrawEnabled"`
	MinWithdraw           int64  `json:"minWithdraw"`
	WithdrawFee           int64  `json:"withdrawFee"`
	ConfirmationsRequired int    `json:"confirmationsRequired"`
	ContractAddress       string `json:"contractAddress,omitempty"`
	Status                int    `json:"status"`
}

// DepositAddress 充值地址
type DepositAddress struct {
	UserID      int64  `json:"userId"`
	Asset       string `json:"asset"`
	Network     string `json:"network"`
	Address     string `json:"address"`
	Tag         string `json:"tag,omitempty"`
	CreatedAtMs int64  `json:"createdAtMs"`
}

// Deposit 充值记录
type Deposit struct {
	DepositID     int64  `json:"depositId"`
	UserID        int64  `json:"userId"`
	Asset         string `json:"asset"`
	Network       string `json:"network"`
	Amount        int64  `json:"amount"`
	Txid          string `json:"txid"`
	Vout          int    `json:"vout"`
	Confirmations int    `json:"confirmations"`
	Status        int    `json:"status"` // 1=PENDING, 2=CONFIRMED, 3=CREDITED
	CreditedAtMs  int64  `json:"creditedAtMs,omitempty"`
	CreatedAtMs   int64  `json:"createdAtMs"`
	UpdatedAtMs   int64  `json:"updatedAtMs"`
}

// Withdrawal 提现记录
type Withdrawal struct {
	WithdrawID     int64  `json:"withdrawId"`
	IdempotencyKey string `json:"idempotencyKey"`
	UserID         int64  `json:"userId"`
	Asset          string `json:"asset"`
	Network        string `json:"network"`
	Amount         int64  `json:"amount"`
	Fee            int64  `json:"fee"`
	Address        string `json:"address"`
	Tag            string `json:"tag,omitempty"`
	Status         int    `json:"status"` // 1=PENDING, 2=APPROVED, 3=REJECTED, 4=PROCESSING, 5=COMPLETED, 6=FAILED
	Txid           string `json:"txid,omitempty"`
	RequestedAtMs  int64  `json:"requestedAtMs"`
	ApprovedAtMs   int64  `json:"approvedAtMs,omitempty"`
	ApprovedBy     int64  `json:"approvedBy,omitempty"`
	SentAtMs       int64  `json:"sentAtMs,omitempty"`
	CompletedAtMs  int64  `json:"completedAtMs,omitempty"`
}

// WithdrawalStatus 提现状态
const (
	WithdrawStatusPending    = 1
	WithdrawStatusApproved   = 2
	WithdrawStatusRejected   = 3
	WithdrawStatusProcessing = 4
	WithdrawStatusCompleted  = 5
	WithdrawStatusFailed     = 6
)

// DepositStatus 充值状态
const (
	DepositStatusPending   = 1
	DepositStatusConfirmed = 2
	DepositStatusCredited  = 3
)

// ListAssets 列出资产
func (r *WalletRepository) ListAssets(ctx context.Context) ([]*Asset, error) {
	query := `SELECT asset, name, precision, status, created_at_ms, updated_at_ms FROM exchange_wallet.assets WHERE status = 1`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []*Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.Asset, &a.Name, &a.Precision, &a.Status, &a.CreatedAtMs, &a.UpdatedAtMs); err != nil {
			return nil, err
		}
		assets = append(assets, &a)
	}
	return assets, nil
}

// ListNetworks 列出网络
func (r *WalletRepository) ListNetworks(ctx context.Context, asset string) ([]*Network, error) {
	query := `
		SELECT asset, network, deposit_enabled, withdraw_enabled, min_withdraw, withdraw_fee,
		       confirmations_required, contract_address, status
		FROM exchange_wallet.networks
		WHERE ($1 = '' OR asset = $1) AND status = 1
	`
	rows, err := r.db.QueryContext(ctx, query, asset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var networks []*Network
	for rows.Next() {
		var n Network
		var contractAddr sql.NullString
		if err := rows.Scan(&n.Asset, &n.Network, &n.DepositEnabled, &n.WithdrawEnabled,
			&n.MinWithdraw, &n.WithdrawFee, &n.ConfirmationsRequired, &contractAddr, &n.Status); err != nil {
			return nil, err
		}
		n.ContractAddress = contractAddr.String
		networks = append(networks, &n)
	}
	return networks, nil
}

// GetNetwork 获取网络配置
func (r *WalletRepository) GetNetwork(ctx context.Context, asset, network string) (*Network, error) {
	query := `
		SELECT asset, network, deposit_enabled, withdraw_enabled, min_withdraw, withdraw_fee,
		       confirmations_required, contract_address, status
		FROM exchange_wallet.networks
		WHERE asset = $1 AND network = $2
	`
	var n Network
	var contractAddr sql.NullString
	err := r.db.QueryRowContext(ctx, query, asset, network).Scan(
		&n.Asset, &n.Network, &n.DepositEnabled, &n.WithdrawEnabled,
		&n.MinWithdraw, &n.WithdrawFee, &n.ConfirmationsRequired, &contractAddr, &n.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.ContractAddress = contractAddr.String
	return &n, nil
}

// GetOrCreateDepositAddress 获取或创建充值地址
func (r *WalletRepository) GetOrCreateDepositAddress(ctx context.Context, userID int64, asset, network, address string) (*DepositAddress, error) {
	// 先查询
	query := `SELECT user_id, asset, network, address, tag, created_at_ms FROM exchange_wallet.deposit_addresses WHERE user_id = $1 AND asset = $2 AND network = $3`
	var addr DepositAddress
	var tag sql.NullString
	err := r.db.QueryRowContext(ctx, query, userID, asset, network).Scan(&addr.UserID, &addr.Asset, &addr.Network, &addr.Address, &tag, &addr.CreatedAtMs)
	if err == nil {
		addr.Tag = tag.String
		return &addr, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// 创建新地址
	now := time.Now().UnixMilli()
	insertQuery := `INSERT INTO exchange_wallet.deposit_addresses (user_id, asset, network, address, created_at_ms) VALUES ($1, $2, $3, $4, $5)`
	_, err = r.db.ExecContext(ctx, insertQuery, userID, asset, network, address, now)
	if err != nil {
		return nil, err
	}

	return &DepositAddress{
		UserID:      userID,
		Asset:       asset,
		Network:     network,
		Address:     address,
		CreatedAtMs: now,
	}, nil
}

// ListDepositAddresses 列出充值地址（用于扫描任务）
func (r *WalletRepository) ListDepositAddresses(ctx context.Context, asset, network string, limit int) ([]*DepositAddress, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	query := `
		SELECT user_id, asset, network, address, tag, created_at_ms
		FROM exchange_wallet.deposit_addresses
		WHERE asset = $1 AND network = $2
		ORDER BY created_at_ms DESC
		LIMIT $3
	`
	rows, err := r.db.QueryContext(ctx, query, asset, network, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DepositAddress
	for rows.Next() {
		var addr DepositAddress
		var tag sql.NullString
		if err := rows.Scan(&addr.UserID, &addr.Asset, &addr.Network, &addr.Address, &tag, &addr.CreatedAtMs); err != nil {
			return nil, err
		}
		addr.Tag = tag.String
		results = append(results, &addr)
	}
	return results, nil
}

// CreateDeposit 创建充值记录
func (r *WalletRepository) CreateDeposit(ctx context.Context, d *Deposit) error {
	now := time.Now().UnixMilli()
	d.CreatedAtMs = now
	d.UpdatedAtMs = now

	query := `
		INSERT INTO exchange_wallet.deposits (deposit_id, user_id, asset, network, amount, txid, vout, confirmations, status, created_at_ms, updated_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (asset, network, txid, vout) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, d.DepositID, d.UserID, d.Asset, d.Network, d.Amount, d.Txid, d.Vout, d.Confirmations, d.Status, d.CreatedAtMs, d.UpdatedAtMs)
	return err
}

// UpsertDeposit 创建或更新充值记录，返回数据库中的记录（用于幂等与确认数更新）
func (r *WalletRepository) UpsertDeposit(ctx context.Context, d *Deposit) (*Deposit, error) {
	now := time.Now().UnixMilli()
	d.CreatedAtMs = now
	d.UpdatedAtMs = now

	query := `
		INSERT INTO exchange_wallet.deposits (
			deposit_id, user_id, asset, network, amount, txid, vout, confirmations, status, created_at_ms, updated_at_ms
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		ON CONFLICT (asset, network, txid, vout) DO UPDATE SET
			confirmations = GREATEST(exchange_wallet.deposits.confirmations, EXCLUDED.confirmations),
			status = CASE
				WHEN exchange_wallet.deposits.status = 3 THEN 3
				WHEN EXCLUDED.status = 3 THEN 3
				WHEN exchange_wallet.deposits.status = 2 OR EXCLUDED.status = 2 THEN 2
				ELSE 1
			END,
			updated_at_ms = EXCLUDED.updated_at_ms
		RETURNING deposit_id, user_id, asset, network, amount, txid, vout, confirmations, status, credited_at_ms, created_at_ms, updated_at_ms
	`

	var out Deposit
	var creditedAt sql.NullInt64
	if err := r.db.QueryRowContext(ctx, query,
		d.DepositID, d.UserID, d.Asset, d.Network, d.Amount, d.Txid, d.Vout, d.Confirmations, d.Status, d.CreatedAtMs, d.UpdatedAtMs,
	).Scan(
		&out.DepositID, &out.UserID, &out.Asset, &out.Network, &out.Amount, &out.Txid, &out.Vout,
		&out.Confirmations, &out.Status, &creditedAt, &out.CreatedAtMs, &out.UpdatedAtMs,
	); err != nil {
		return nil, err
	}
	out.CreditedAtMs = creditedAt.Int64
	return &out, nil
}

// UpdateDepositStatus 更新充值状态
func (r *WalletRepository) UpdateDepositStatus(ctx context.Context, depositID int64, status, confirmations int) error {
	now := time.Now().UnixMilli()
	var query string
	if status == DepositStatusCredited {
		query = `UPDATE exchange_wallet.deposits SET status = $1, confirmations = $2, credited_at_ms = $3, updated_at_ms = $3 WHERE deposit_id = $4`
	} else {
		query = `UPDATE exchange_wallet.deposits SET status = $1, confirmations = $2, updated_at_ms = $3 WHERE deposit_id = $4`
	}
	_, err := r.db.ExecContext(ctx, query, status, confirmations, now, depositID)
	return err
}

// ListDeposits 列出充值记录
func (r *WalletRepository) ListDeposits(ctx context.Context, userID int64, limit int) ([]*Deposit, error) {
	query := `
		SELECT deposit_id, user_id, asset, network, amount, txid, vout, confirmations, status, credited_at_ms, created_at_ms, updated_at_ms
		FROM exchange_wallet.deposits
		WHERE user_id = $1
		ORDER BY created_at_ms DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deposits []*Deposit
	for rows.Next() {
		var d Deposit
		var creditedAt sql.NullInt64
		if err := rows.Scan(&d.DepositID, &d.UserID, &d.Asset, &d.Network, &d.Amount, &d.Txid, &d.Vout, &d.Confirmations, &d.Status, &creditedAt, &d.CreatedAtMs, &d.UpdatedAtMs); err != nil {
			return nil, err
		}
		d.CreditedAtMs = creditedAt.Int64
		deposits = append(deposits, &d)
	}
	return deposits, nil
}

// CreateWithdrawal 创建提现记录
func (r *WalletRepository) CreateWithdrawal(ctx context.Context, w *Withdrawal) error {
	now := time.Now().UnixMilli()
	w.RequestedAtMs = now

	query := `
		INSERT INTO exchange_wallet.withdrawals (withdraw_id, idempotency_key, user_id, asset, network, amount, fee, address, tag, status, requested_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.ExecContext(ctx, query, w.WithdrawID, w.IdempotencyKey, w.UserID, w.Asset, w.Network, w.Amount, w.Fee, w.Address, nullString(w.Tag), w.Status, w.RequestedAtMs)
	return err
}

// GetWithdrawalByIdempotencyKey 通过幂等键获取提现
func (r *WalletRepository) GetWithdrawalByIdempotencyKey(ctx context.Context, key string) (*Withdrawal, error) {
	query := `
		SELECT withdraw_id, idempotency_key, user_id, asset, network, amount, fee, address, tag, status, txid, requested_at_ms, approved_at_ms, approved_by, sent_at_ms, completed_at_ms
		FROM exchange_wallet.withdrawals
		WHERE idempotency_key = $1
	`
	return r.scanWithdrawal(r.db.QueryRowContext(ctx, query, key))
}

// GetWithdrawal 获取提现记录
func (r *WalletRepository) GetWithdrawal(ctx context.Context, withdrawID int64) (*Withdrawal, error) {
	query := `
		SELECT withdraw_id, idempotency_key, user_id, asset, network, amount, fee, address, tag, status, txid, requested_at_ms, approved_at_ms, approved_by, sent_at_ms, completed_at_ms
		FROM exchange_wallet.withdrawals
		WHERE withdraw_id = $1
	`
	return r.scanWithdrawal(r.db.QueryRowContext(ctx, query, withdrawID))
}

// UpdateWithdrawalStatus 更新提现状态
func (r *WalletRepository) UpdateWithdrawalStatus(ctx context.Context, withdrawID int64, status int, approvedBy int64, txid string) error {
	now := time.Now().UnixMilli()
	var query string
	switch status {
	case WithdrawStatusApproved:
		query = `UPDATE exchange_wallet.withdrawals SET status = $1, approved_at_ms = $2, approved_by = $3 WHERE withdraw_id = $4`
		_, err := r.db.ExecContext(ctx, query, status, now, approvedBy, withdrawID)
		return err
	case WithdrawStatusRejected:
		query = `UPDATE exchange_wallet.withdrawals SET status = $1, approved_at_ms = $2, approved_by = $3 WHERE withdraw_id = $4`
		_, err := r.db.ExecContext(ctx, query, status, now, approvedBy, withdrawID)
		return err
	case WithdrawStatusProcessing:
		query = `UPDATE exchange_wallet.withdrawals SET status = $1, sent_at_ms = $2 WHERE withdraw_id = $3`
		_, err := r.db.ExecContext(ctx, query, status, now, withdrawID)
		return err
	case WithdrawStatusCompleted:
		query = `UPDATE exchange_wallet.withdrawals SET status = $1, txid = $2, completed_at_ms = $3 WHERE withdraw_id = $4`
		_, err := r.db.ExecContext(ctx, query, status, txid, now, withdrawID)
		return err
	default:
		query = `UPDATE exchange_wallet.withdrawals SET status = $1 WHERE withdraw_id = $2`
		_, err := r.db.ExecContext(ctx, query, status, withdrawID)
		return err
	}
}

// UpdateWithdrawalStatusCAS 条件更新提现状态，返回是否命中并更新成功
func (r *WalletRepository) UpdateWithdrawalStatusCAS(ctx context.Context, withdrawID int64, expectedStatuses []int, status int, approvedBy int64, txid string) (bool, error) {
	if withdrawID <= 0 {
		return false, errors.New("invalid withdrawID")
	}
	if len(expectedStatuses) == 0 {
		return false, errors.New("expected statuses required")
	}

	now := time.Now().UnixMilli()
	var (
		query string
		args  []interface{}
	)
	switch status {
	case WithdrawStatusApproved:
		query = `
			UPDATE exchange_wallet.withdrawals
			SET status = $1, approved_at_ms = $2, approved_by = $3
			WHERE withdraw_id = $4 AND status = ANY($5)
		`
		args = []interface{}{status, now, approvedBy, withdrawID, pq.Array(expectedStatuses)}
	case WithdrawStatusRejected:
		query = `
			UPDATE exchange_wallet.withdrawals
			SET status = $1, approved_at_ms = $2, approved_by = $3
			WHERE withdraw_id = $4 AND status = ANY($5)
		`
		args = []interface{}{status, now, approvedBy, withdrawID, pq.Array(expectedStatuses)}
	case WithdrawStatusProcessing:
		query = `
			UPDATE exchange_wallet.withdrawals
			SET status = $1, sent_at_ms = $2
			WHERE withdraw_id = $3 AND status = ANY($4)
		`
		args = []interface{}{status, now, withdrawID, pq.Array(expectedStatuses)}
	case WithdrawStatusCompleted:
		query = `
			UPDATE exchange_wallet.withdrawals
			SET status = $1, txid = $2, completed_at_ms = $3
			WHERE withdraw_id = $4 AND status = ANY($5)
		`
		args = []interface{}{status, txid, now, withdrawID, pq.Array(expectedStatuses)}
	default:
		query = `
			UPDATE exchange_wallet.withdrawals
			SET status = $1
			WHERE withdraw_id = $2 AND status = ANY($3)
		`
		args = []interface{}{status, withdrawID, pq.Array(expectedStatuses)}
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ListWithdrawals 列出提现记录
func (r *WalletRepository) ListWithdrawals(ctx context.Context, userID int64, limit int) ([]*Withdrawal, error) {
	query := `
		SELECT withdraw_id, idempotency_key, user_id, asset, network, amount, fee, address, tag, status, txid, requested_at_ms, approved_at_ms, approved_by, sent_at_ms, completed_at_ms
		FROM exchange_wallet.withdrawals
		WHERE user_id = $1
		ORDER BY requested_at_ms DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []*Withdrawal
	for rows.Next() {
		w, err := r.scanWithdrawalRow(rows)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}
	return withdrawals, nil
}

// ListPendingWithdrawals 列出待审核提现
func (r *WalletRepository) ListPendingWithdrawals(ctx context.Context, limit int) ([]*Withdrawal, error) {
	query := `
		SELECT withdraw_id, idempotency_key, user_id, asset, network, amount, fee, address, tag, status, txid, requested_at_ms, approved_at_ms, approved_by, sent_at_ms, completed_at_ms
		FROM exchange_wallet.withdrawals
		WHERE status = $1
		ORDER BY requested_at_ms
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, WithdrawStatusPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []*Withdrawal
	for rows.Next() {
		w, err := r.scanWithdrawalRow(rows)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}
	return withdrawals, nil
}

func (r *WalletRepository) scanWithdrawal(row *sql.Row) (*Withdrawal, error) {
	var w Withdrawal
	var tag, txid sql.NullString
	var approvedAt, approvedBy, sentAt, completedAt sql.NullInt64

	err := row.Scan(&w.WithdrawID, &w.IdempotencyKey, &w.UserID, &w.Asset, &w.Network, &w.Amount, &w.Fee, &w.Address, &tag, &w.Status, &txid, &w.RequestedAtMs, &approvedAt, &approvedBy, &sentAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	w.Tag = tag.String
	w.Txid = txid.String
	w.ApprovedAtMs = approvedAt.Int64
	w.ApprovedBy = approvedBy.Int64
	w.SentAtMs = sentAt.Int64
	w.CompletedAtMs = completedAt.Int64
	return &w, nil
}

func (r *WalletRepository) scanWithdrawalRow(rows *sql.Rows) (*Withdrawal, error) {
	var w Withdrawal
	var tag, txid sql.NullString
	var approvedAt, approvedBy, sentAt, completedAt sql.NullInt64

	err := rows.Scan(&w.WithdrawID, &w.IdempotencyKey, &w.UserID, &w.Asset, &w.Network, &w.Amount, &w.Fee, &w.Address, &tag, &w.Status, &txid, &w.RequestedAtMs, &approvedAt, &approvedBy, &sentAt, &completedAt)
	if err != nil {
		return nil, err
	}

	w.Tag = tag.String
	w.Txid = txid.String
	w.ApprovedAtMs = approvedAt.Int64
	w.ApprovedBy = approvedBy.Int64
	w.SentAtMs = sentAt.Int64
	w.CompletedAtMs = completedAt.Int64
	return &w, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
