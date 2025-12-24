// Package repository 订单数据访问层
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

var ErrDuplicateTrade = errors.New("duplicate trade")

// TradeRepository 成交仓储
type TradeRepository struct {
	db *sql.DB
}

// NewTradeRepository 创建成交仓储
func NewTradeRepository(db *sql.DB) *TradeRepository {
	return &TradeRepository{db: db}
}

// SaveTrade 保存成交记录
func (r *TradeRepository) SaveTrade(ctx context.Context, trade *Trade) error {
	query := `
		INSERT INTO exchange_order.trades
		(trade_id, symbol, maker_order_id, taker_order_id, maker_user_id, taker_user_id,
		 price, qty, quote_qty, maker_fee, taker_fee, fee_asset, taker_side, timestamp_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.db.ExecContext(ctx, query,
		trade.TradeID, trade.Symbol, trade.MakerOrderID, trade.TakerOrderID,
		trade.MakerUserID, trade.TakerUserID,
		strconv.FormatInt(trade.Price, 10),
		strconv.FormatInt(trade.Qty, 10),
		strconv.FormatInt(trade.QuoteQty, 10),
		strconv.FormatInt(trade.MakerFee, 10),
		strconv.FormatInt(trade.TakerFee, 10),
		trade.FeeAsset, trade.TakerSide, trade.TimestampMs,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateTrade
		}
		return fmt.Errorf("insert trade: %w", err)
	}
	return nil
}

// ListTradesByUser returns recent trades where the given user participated as maker or taker.
func (r *TradeRepository) ListTradesByUser(ctx context.Context, userID int64, symbol string, startTimeMs, endTimeMs int64, limit int) ([]*Trade, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT trade_id, symbol, maker_order_id, taker_order_id, maker_user_id, taker_user_id,
			   price, qty, quote_qty, maker_fee, taker_fee, fee_asset, taker_side, timestamp_ms
		FROM exchange_order.trades
		WHERE (maker_user_id = $1 OR taker_user_id = $1)
		  AND ($2 = '' OR symbol = $2)
		  AND ($3 = 0 OR timestamp_ms >= $3)
		  AND ($4 = 0 OR timestamp_ms <= $4)
		ORDER BY timestamp_ms DESC
		LIMIT $5
	`

	rows, err := r.db.QueryContext(ctx, query, userID, symbol, startTimeMs, endTimeMs, limit)
	if err != nil {
		return nil, fmt.Errorf("query trades: %w", err)
	}
	defer rows.Close()

	var trades []*Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(
			&t.TradeID, &t.Symbol, &t.MakerOrderID, &t.TakerOrderID, &t.MakerUserID, &t.TakerUserID,
			&t.Price, &t.Qty, &t.QuoteQty, &t.MakerFee, &t.TakerFee, &t.FeeAsset, &t.TakerSide, &t.TimestampMs,
		); err != nil {
			return nil, fmt.Errorf("scan trade: %w", err)
		}
		trades = append(trades, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows trade: %w", err)
	}
	return trades, nil
}
