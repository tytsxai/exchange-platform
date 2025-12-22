// Package repository 后台数据访问层
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AdminRepository 后台仓储
type AdminRepository struct {
	db *sql.DB
}

// NewAdminRepository 创建仓储
func NewAdminRepository(db *sql.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// SymbolConfig 交易对配置
type SymbolConfig struct {
	Symbol         string  `json:"symbol"`
	BaseAsset      string  `json:"baseAsset"`
	QuoteAsset     string  `json:"quoteAsset"`
	PriceTick      int64   `json:"priceTick"`
	QtyStep        int64   `json:"qtyStep"`
	PricePrecision int     `json:"pricePrecision"`
	QtyPrecision   int     `json:"qtyPrecision"`
	MinQty         int64   `json:"minQty"`
	MaxQty         int64   `json:"maxQty"`
	MinNotional    int64   `json:"minNotional"`
	PriceLimitRate float64 `json:"priceLimitRate"`
	MakerFeeRate   float64 `json:"makerFeeRate"`
	TakerFeeRate   float64 `json:"takerFeeRate"`
	Status         int     `json:"status"` // 1=TRADING, 2=HALT, 3=CANCEL_ONLY
	CreatedAtMs    int64   `json:"createdAtMs"`
	UpdatedAtMs    int64   `json:"updatedAtMs"`
}

// ListSymbolConfigs 列出所有交易对
func (r *AdminRepository) ListSymbolConfigs(ctx context.Context) ([]*SymbolConfig, error) {
	query := `
		SELECT symbol, base_asset, quote_asset, price_tick, qty_step,
		       price_precision, qty_precision, min_qty, max_qty, min_notional,
		       price_limit_rate, maker_fee_rate, taker_fee_rate, status,
		       created_at_ms, updated_at_ms
		FROM exchange_order.symbol_configs
		ORDER BY symbol
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query symbols: %w", err)
	}
	defer rows.Close()

	var configs []*SymbolConfig
	for rows.Next() {
		var c SymbolConfig
		if err := rows.Scan(
			&c.Symbol, &c.BaseAsset, &c.QuoteAsset, &c.PriceTick, &c.QtyStep,
			&c.PricePrecision, &c.QtyPrecision, &c.MinQty, &c.MaxQty, &c.MinNotional,
			&c.PriceLimitRate, &c.MakerFeeRate, &c.TakerFeeRate, &c.Status,
			&c.CreatedAtMs, &c.UpdatedAtMs,
		); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		configs = append(configs, &c)
	}
	return configs, nil
}

// GetSymbolConfig 获取交易对配置
func (r *AdminRepository) GetSymbolConfig(ctx context.Context, symbol string) (*SymbolConfig, error) {
	query := `
		SELECT symbol, base_asset, quote_asset, price_tick, qty_step,
		       price_precision, qty_precision, min_qty, max_qty, min_notional,
		       price_limit_rate, maker_fee_rate, taker_fee_rate, status,
		       created_at_ms, updated_at_ms
		FROM exchange_order.symbol_configs
		WHERE symbol = $1
	`
	var c SymbolConfig
	err := r.db.QueryRowContext(ctx, query, symbol).Scan(
		&c.Symbol, &c.BaseAsset, &c.QuoteAsset, &c.PriceTick, &c.QtyStep,
		&c.PricePrecision, &c.QtyPrecision, &c.MinQty, &c.MaxQty, &c.MinNotional,
		&c.PriceLimitRate, &c.MakerFeeRate, &c.TakerFeeRate, &c.Status,
		&c.CreatedAtMs, &c.UpdatedAtMs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get symbol: %w", err)
	}
	return &c, nil
}

// CreateSymbolConfig 创建交易对
func (r *AdminRepository) CreateSymbolConfig(ctx context.Context, c *SymbolConfig) error {
	now := time.Now().UnixMilli()
	c.CreatedAtMs = now
	c.UpdatedAtMs = now

	query := `
		INSERT INTO exchange_order.symbol_configs
		(symbol, base_asset, quote_asset, price_tick, qty_step,
		 price_precision, qty_precision, min_qty, max_qty, min_notional,
		 price_limit_rate, maker_fee_rate, taker_fee_rate, status,
		 created_at_ms, updated_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	_, err := r.db.ExecContext(ctx, query,
		c.Symbol, c.BaseAsset, c.QuoteAsset, c.PriceTick, c.QtyStep,
		c.PricePrecision, c.QtyPrecision, c.MinQty, c.MaxQty, c.MinNotional,
		c.PriceLimitRate, c.MakerFeeRate, c.TakerFeeRate, c.Status,
		c.CreatedAtMs, c.UpdatedAtMs,
	)
	return err
}

// UpdateSymbolConfig 更新交易对
func (r *AdminRepository) UpdateSymbolConfig(ctx context.Context, c *SymbolConfig) error {
	c.UpdatedAtMs = time.Now().UnixMilli()

	query := `
		UPDATE exchange_order.symbol_configs
		SET price_tick = $1, qty_step = $2, price_precision = $3, qty_precision = $4,
		    min_qty = $5, max_qty = $6, min_notional = $7, price_limit_rate = $8,
		    maker_fee_rate = $9, taker_fee_rate = $10, status = $11, updated_at_ms = $12
		WHERE symbol = $13
	`
	result, err := r.db.ExecContext(ctx, query,
		c.PriceTick, c.QtyStep, c.PricePrecision, c.QtyPrecision,
		c.MinQty, c.MaxQty, c.MinNotional, c.PriceLimitRate,
		c.MakerFeeRate, c.TakerFeeRate, c.Status, c.UpdatedAtMs,
		c.Symbol,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("symbol not found")
	}
	return nil
}

// UpdateSymbolStatus 更新交易对状态（Kill Switch）
func (r *AdminRepository) UpdateSymbolStatus(ctx context.Context, symbol string, status int) error {
	query := `
		UPDATE exchange_order.symbol_configs
		SET status = $1, updated_at_ms = $2
		WHERE symbol = $3
	`
	_, err := r.db.ExecContext(ctx, query, status, time.Now().UnixMilli(), symbol)
	return err
}

// UpdateAllSymbolStatus 更新所有交易对状态（全局 Kill Switch）
func (r *AdminRepository) UpdateAllSymbolStatus(ctx context.Context, status int) error {
	query := `
		UPDATE exchange_order.symbol_configs
		SET status = $1, updated_at_ms = $2
	`
	_, err := r.db.ExecContext(ctx, query, status, time.Now().UnixMilli())
	return err
}

// AuditLog 审计日志
type AuditLog struct {
	AuditID     int64           `json:"auditId"`
	ActorUserID int64           `json:"actorUserId"`
	Action      string          `json:"action"`
	TargetType  string          `json:"targetType"`
	TargetID    string          `json:"targetId"`
	BeforeJSON  json.RawMessage `json:"beforeJson,omitempty"`
	AfterJSON   json.RawMessage `json:"afterJson,omitempty"`
	IP          string          `json:"ip"`
	CreatedAtMs int64           `json:"createdAtMs"`
}

// CreateAuditLog 创建审计日志
func (r *AdminRepository) CreateAuditLog(ctx context.Context, log *AuditLog) error {
	log.CreatedAtMs = time.Now().UnixMilli()

	query := `
		INSERT INTO exchange_admin.audit_logs
		(audit_id, actor_user_id, action, target_type, target_id, before_json, after_json, ip, created_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query,
		log.AuditID, log.ActorUserID, log.Action, log.TargetType, log.TargetID,
		log.BeforeJSON, log.AfterJSON, log.IP, log.CreatedAtMs,
	)
	return err
}

// ListAuditLogs 查询审计日志
func (r *AdminRepository) ListAuditLogs(ctx context.Context, targetType string, limit int) ([]*AuditLog, error) {
	query := `
		SELECT audit_id, actor_user_id, action, target_type, target_id,
		       before_json, after_json, ip, created_at_ms
		FROM exchange_admin.audit_logs
		WHERE ($1 = '' OR target_type = $1)
		ORDER BY created_at_ms DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, targetType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		var l AuditLog
		var beforeJSON, afterJSON sql.NullString
		if err := rows.Scan(
			&l.AuditID, &l.ActorUserID, &l.Action, &l.TargetType, &l.TargetID,
			&beforeJSON, &afterJSON, &l.IP, &l.CreatedAtMs,
		); err != nil {
			return nil, err
		}
		if beforeJSON.Valid {
			l.BeforeJSON = json.RawMessage(beforeJSON.String)
		}
		if afterJSON.Valid {
			l.AfterJSON = json.RawMessage(afterJSON.String)
		}
		logs = append(logs, &l)
	}
	return logs, nil
}

// Role 角色
type Role struct {
	RoleID      int64    `json:"roleId"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	CreatedAtMs int64    `json:"createdAtMs"`
	UpdatedAtMs int64    `json:"updatedAtMs"`
}

// ListRoles 列出角色
func (r *AdminRepository) ListRoles(ctx context.Context) ([]*Role, error) {
	query := `
		SELECT role_id, name, permissions, created_at_ms, updated_at_ms
		FROM exchange_admin.roles
		ORDER BY role_id
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		var role Role
		var perms string
		if err := rows.Scan(&role.RoleID, &role.Name, &perms, &role.CreatedAtMs, &role.UpdatedAtMs); err != nil {
			return nil, err
		}
		// 简化：实际应该解析 PostgreSQL 数组
		role.Permissions = []string{perms}
		roles = append(roles, &role)
	}
	return roles, nil
}

// GetUserRoles 获取用户角色
func (r *AdminRepository) GetUserRoles(ctx context.Context, userID int64) ([]int64, error) {
	query := `SELECT role_id FROM exchange_admin.user_roles WHERE user_id = $1`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roleIDs []int64
	for rows.Next() {
		var roleID int64
		if err := rows.Scan(&roleID); err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, roleID)
	}
	return roleIDs, nil
}

// AssignUserRole 分配用户角色
func (r *AdminRepository) AssignUserRole(ctx context.Context, userID, roleID int64) error {
	query := `
		INSERT INTO exchange_admin.user_roles (user_id, role_id, created_at_ms)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, role_id) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, userID, roleID, time.Now().UnixMilli())
	return err
}

// RemoveUserRole 移除用户角色
func (r *AdminRepository) RemoveUserRole(ctx context.Context, userID, roleID int64) error {
	query := `DELETE FROM exchange_admin.user_roles WHERE user_id = $1 AND role_id = $2`
	_, err := r.db.ExecContext(ctx, query, userID, roleID)
	return err
}
