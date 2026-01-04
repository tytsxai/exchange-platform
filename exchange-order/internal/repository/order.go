// Package repository 订单数据访问层
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrDuplicateClientOrderID = errors.New("duplicate client order id")
)

// OrderStatus 订单状态
const (
	StatusInit            = 0
	StatusNew             = 1
	StatusPartiallyFilled = 2
	StatusFilled          = 3
	StatusCanceled        = 4
	StatusRejected        = 5
	StatusExpired         = 6
)

// Side 订单方向
const (
	SideBuy  = 1
	SideSell = 2
)

// OrderType 订单类型
const (
	TypeLimit  = 1
	TypeMarket = 2
)

// Order 订单
type Order struct {
	OrderID            int64
	ClientOrderID      string
	UserID             int64
	Symbol             string
	Side               int
	Type               int
	TimeInForce        int
	Price              string // DECIMAL from DB
	StopPrice          string // DECIMAL from DB
	OrigQty            string // DECIMAL from DB
	ExecutedQty        string // DECIMAL from DB
	CumulativeQuoteQty string // DECIMAL from DB
	Status             int
	RejectReason       string
	CancelReason       string
	CreateTimeMs       int64
	UpdateTimeMs       int64
	TransactTimeMs     int64
}

// Trade 成交
type Trade struct {
	TradeID      int64
	Symbol       string
	MakerOrderID int64
	TakerOrderID int64
	MakerUserID  int64
	TakerUserID  int64
	Price        int64
	Qty          int64
	QuoteQty     int64
	MakerFee     int64
	TakerFee     int64
	FeeAsset     string
	TakerSide    int
	TimestampMs  int64
}

// SymbolConfig 交易对配置
type SymbolConfig struct {
	Symbol         string
	BaseAsset      string
	QuoteAsset     string
	PriceTick      string // DECIMAL from DB
	QtyStep        string // DECIMAL from DB
	PricePrecision int
	QtyPrecision   int
	BasePrecision  int    `json:"-"`
	QuotePrecision int    `json:"-"`
	MinQty         string // DECIMAL from DB
	MaxQty         string // DECIMAL from DB
	MinNotional    string // DECIMAL from DB
	PriceLimitRate string // DECIMAL from DB
	MakerFeeRate   string // DECIMAL from DB
	TakerFeeRate   string // DECIMAL from DB
	Status         int    // 1=TRADING, 2=HALT, 3=CANCEL_ONLY
}

// OrderRepository 订单仓储
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository 创建仓储
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// CreateOrder 创建订单
func (r *OrderRepository) CreateOrder(ctx context.Context, order *Order) error {
	query := `
		INSERT INTO exchange_order.orders
		(order_id, client_order_id, user_id, symbol, side, type, time_in_force,
		 price, stop_price, orig_qty, executed_qty, cumulative_quote_qty, status,
		 reject_reason, cancel_reason, create_time_ms, update_time_ms, transact_time_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`
	_, err := r.db.ExecContext(ctx, query,
		order.OrderID, nullString(order.ClientOrderID), order.UserID, order.Symbol,
		order.Side, order.Type, order.TimeInForce, order.Price, order.StopPrice,
		order.OrigQty, order.ExecutedQty, order.CumulativeQuoteQty, order.Status,
		order.RejectReason, order.CancelReason, order.CreateTimeMs, order.UpdateTimeMs,
		nullInt64(order.TransactTimeMs),
	)
	if err != nil {
		// 检查唯一约束冲突
		if isUniqueViolation(err) {
			return ErrDuplicateClientOrderID
		}
		return fmt.Errorf("insert order: %w", err)
	}
	return nil
}

// GetOrder 获取订单
func (r *OrderRepository) GetOrder(ctx context.Context, orderID int64) (*Order, error) {
	query := `
		SELECT order_id, client_order_id, user_id, symbol, side, type, time_in_force,
		       price, stop_price, orig_qty, executed_qty, cumulative_quote_qty, status,
		       reject_reason, cancel_reason, create_time_ms, update_time_ms, transact_time_ms
		FROM exchange_order.orders
		WHERE order_id = $1
	`
	return r.scanOrder(r.db.QueryRowContext(ctx, query, orderID))
}

// GetOrderByClientID 通过 clientOrderId 获取订单
func (r *OrderRepository) GetOrderByClientID(ctx context.Context, userID int64, clientOrderID string) (*Order, error) {
	query := `
		SELECT order_id, client_order_id, user_id, symbol, side, type, time_in_force,
		       price, stop_price, orig_qty, executed_qty, cumulative_quote_qty, status,
		       reject_reason, cancel_reason, create_time_ms, update_time_ms, transact_time_ms
		FROM exchange_order.orders
		WHERE user_id = $1 AND client_order_id = $2
	`
	return r.scanOrder(r.db.QueryRowContext(ctx, query, userID, clientOrderID))
}

// UpdateOrderStatus 更新订单状态
func (r *OrderRepository) UpdateOrderStatus(ctx context.Context, orderID int64, status int, executedQty, cumulativeQuoteQty, updateTimeMs int64) error {
	query := `
		UPDATE exchange_order.orders
		SET status = $1, executed_qty = $2, cumulative_quote_qty = $3, update_time_ms = $4
		WHERE order_id = $5
	`
	result, err := r.db.ExecContext(ctx, query, status, executedQty, cumulativeQuoteQty, updateTimeMs, orderID)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// AddOrderCumulativeQuoteQty 增量更新累计成交额
func (r *OrderRepository) AddOrderCumulativeQuoteQty(ctx context.Context, orderID int64, delta int64, updateTimeMs int64) error {
	query := `
		UPDATE exchange_order.orders
		SET cumulative_quote_qty = cumulative_quote_qty + $1, update_time_ms = $2
		WHERE order_id = $3
	`
	result, err := r.db.ExecContext(ctx, query, delta, updateTimeMs, orderID)
	if err != nil {
		return fmt.Errorf("update cumulative quote qty: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// CancelOrder 取消订单
func (r *OrderRepository) CancelOrder(ctx context.Context, orderID int64, reason string, updateTimeMs int64) error {
	query := `
		UPDATE exchange_order.orders
		SET status = $1, cancel_reason = $2, update_time_ms = $3
		WHERE order_id = $4 AND status IN (1, 2)
	`
	result, err := r.db.ExecContext(ctx, query, StatusCanceled, reason, updateTimeMs, orderID)
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// RejectOrder 拒绝订单
func (r *OrderRepository) RejectOrder(ctx context.Context, orderID int64, reason string, updateTimeMs int64) error {
	query := `
		UPDATE exchange_order.orders
		SET status = $1, reject_reason = $2, update_time_ms = $3
		WHERE order_id = $4 AND status IN (0, 1, 2)
	`
	result, err := r.db.ExecContext(ctx, query, StatusRejected, reason, updateTimeMs, orderID)
	if err != nil {
		return fmt.Errorf("reject order: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// ListOpenOrders 查询当前委托
func (r *OrderRepository) ListOpenOrders(ctx context.Context, userID int64, symbol string, limit int) ([]*Order, error) {
	query := `
		SELECT order_id, client_order_id, user_id, symbol, side, type, time_in_force,
		       price, stop_price, orig_qty, executed_qty, cumulative_quote_qty, status,
		       reject_reason, cancel_reason, create_time_ms, update_time_ms, transact_time_ms
		FROM exchange_order.orders
		WHERE user_id = $1 AND status IN (1, 2)
		  AND ($2 = '' OR symbol = $2)
		ORDER BY create_time_ms DESC
		LIMIT $3
	`
	return r.queryOrders(ctx, query, userID, symbol, limit)
}

// ListOrders 查询历史订单
func (r *OrderRepository) ListOrders(ctx context.Context, userID int64, symbol string, startTime, endTime int64, limit int) ([]*Order, error) {
	query := `
		SELECT order_id, client_order_id, user_id, symbol, side, type, time_in_force,
		       price, stop_price, orig_qty, executed_qty, cumulative_quote_qty, status,
		       reject_reason, cancel_reason, create_time_ms, update_time_ms, transact_time_ms
		FROM exchange_order.orders
		WHERE user_id = $1
		  AND ($2 = '' OR symbol = $2)
		  AND create_time_ms >= $3 AND create_time_ms <= $4
		ORDER BY create_time_ms DESC
		LIMIT $5
	`
	return r.queryOrders(ctx, query, userID, symbol, startTime, endTime, limit)
}

// GetSymbolConfig 获取交易对配置
func (r *OrderRepository) GetSymbolConfig(ctx context.Context, symbol string) (*SymbolConfig, error) {
	query := `
		SELECT sc.symbol, sc.base_asset, sc.quote_asset, sc.price_tick, sc.qty_step,
		       sc.price_precision, sc.qty_precision, sc.min_qty, sc.max_qty, sc.min_notional,
		       sc.price_limit_rate, sc.maker_fee_rate, sc.taker_fee_rate, sc.status,
		       COALESCE(base.precision, 0) AS base_precision,
		       COALESCE(quote.precision, 0) AS quote_precision
		FROM exchange_order.symbol_configs sc
		LEFT JOIN exchange_wallet.assets base ON base.asset = sc.base_asset
		LEFT JOIN exchange_wallet.assets quote ON quote.asset = sc.quote_asset
		WHERE sc.symbol = $1
	`
	var cfg SymbolConfig
	err := r.db.QueryRowContext(ctx, query, symbol).Scan(
		&cfg.Symbol, &cfg.BaseAsset, &cfg.QuoteAsset, &cfg.PriceTick, &cfg.QtyStep,
		&cfg.PricePrecision, &cfg.QtyPrecision, &cfg.MinQty, &cfg.MaxQty, &cfg.MinNotional,
		&cfg.PriceLimitRate, &cfg.MakerFeeRate, &cfg.TakerFeeRate, &cfg.Status,
		&cfg.BasePrecision, &cfg.QuotePrecision,
	)
	if err == sql.ErrNoRows {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get symbol config: %w", err)
	}
	return &cfg, nil
}

// ListSymbolConfigs 获取所有交易对配置
func (r *OrderRepository) ListSymbolConfigs(ctx context.Context) ([]*SymbolConfig, error) {
	query := `
		SELECT sc.symbol, sc.base_asset, sc.quote_asset, sc.price_tick, sc.qty_step,
		       sc.price_precision, sc.qty_precision, sc.min_qty, sc.max_qty, sc.min_notional,
		       sc.price_limit_rate, sc.maker_fee_rate, sc.taker_fee_rate, sc.status,
		       COALESCE(base.precision, 0) AS base_precision,
		       COALESCE(quote.precision, 0) AS quote_precision
		FROM exchange_order.symbol_configs sc
		LEFT JOIN exchange_wallet.assets base ON base.asset = sc.base_asset
		LEFT JOIN exchange_wallet.assets quote ON quote.asset = sc.quote_asset
		WHERE sc.status = 1
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list symbol configs: %w", err)
	}
	defer rows.Close()

	var configs []*SymbolConfig
	for rows.Next() {
		var cfg SymbolConfig
		if err := rows.Scan(
			&cfg.Symbol, &cfg.BaseAsset, &cfg.QuoteAsset, &cfg.PriceTick, &cfg.QtyStep,
			&cfg.PricePrecision, &cfg.QtyPrecision, &cfg.MinQty, &cfg.MaxQty, &cfg.MinNotional,
			&cfg.PriceLimitRate, &cfg.MakerFeeRate, &cfg.TakerFeeRate, &cfg.Status,
			&cfg.BasePrecision, &cfg.QuotePrecision,
		); err != nil {
			return nil, fmt.Errorf("scan symbol config: %w", err)
		}
		configs = append(configs, &cfg)
	}
	return configs, nil
}

func (r *OrderRepository) scanOrder(row *sql.Row) (*Order, error) {
	var o Order
	var clientOrderID, rejectReason, cancelReason sql.NullString
	var transactTimeMs sql.NullInt64

	err := row.Scan(
		&o.OrderID, &clientOrderID, &o.UserID, &o.Symbol, &o.Side, &o.Type, &o.TimeInForce,
		&o.Price, &o.StopPrice, &o.OrigQty, &o.ExecutedQty, &o.CumulativeQuoteQty, &o.Status,
		&rejectReason, &cancelReason, &o.CreateTimeMs, &o.UpdateTimeMs, &transactTimeMs,
	)
	if err == sql.ErrNoRows {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan order: %w", err)
	}

	o.ClientOrderID = clientOrderID.String
	o.RejectReason = rejectReason.String
	o.CancelReason = cancelReason.String
	o.TransactTimeMs = transactTimeMs.Int64

	return &o, nil
}

func (r *OrderRepository) queryOrders(ctx context.Context, query string, args ...interface{}) ([]*Order, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		var o Order
		var clientOrderID, rejectReason, cancelReason sql.NullString
		var transactTimeMs sql.NullInt64

		if err := rows.Scan(
			&o.OrderID, &clientOrderID, &o.UserID, &o.Symbol, &o.Side, &o.Type, &o.TimeInForce,
			&o.Price, &o.StopPrice, &o.OrigQty, &o.ExecutedQty, &o.CumulativeQuoteQty, &o.Status,
			&rejectReason, &cancelReason, &o.CreateTimeMs, &o.UpdateTimeMs, &transactTimeMs,
		); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}

		o.ClientOrderID = clientOrderID.String
		o.RejectReason = rejectReason.String
		o.CancelReason = cancelReason.String
		o.TransactTimeMs = transactTimeMs.Int64

		orders = append(orders, &o)
	}
	return orders, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: i, Valid: true}
}

func isUniqueViolation(err error) bool {
	return err != nil && (contains(err.Error(), "unique") || contains(err.Error(), "duplicate"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
