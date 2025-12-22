// Package repository 订单数据访问层
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

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
		return fmt.Errorf("insert trade: %w", err)
	}
	return nil
}
