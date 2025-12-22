package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTradeRepository_SaveTrade(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewTradeRepository(db)
	trade := &Trade{
		TradeID:      101,
		Symbol:       "BTCUSDT",
		MakerOrderID: 200,
		TakerOrderID: 201,
		MakerUserID:  11,
		TakerUserID:  12,
		Price:        30000 * 1e8,
		Qty:          2 * 1e8,
		QuoteQty:     60000 * 1e8,
		MakerFee:     0,
		TakerFee:     0,
		FeeAsset:     "USDT",
		TakerSide:    SideBuy,
		TimestampMs:  1234567890,
	}

	query := regexp.QuoteMeta(`
		INSERT INTO exchange_order.trades
		(trade_id, symbol, maker_order_id, taker_order_id, maker_user_id, taker_user_id,
		 price, qty, quote_qty, maker_fee, taker_fee, fee_asset, taker_side, timestamp_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`)

	mock.ExpectExec(query).
		WithArgs(
			trade.TradeID, trade.Symbol, trade.MakerOrderID, trade.TakerOrderID,
			trade.MakerUserID, trade.TakerUserID,
			"3000000000000", "200000000", "6000000000000", "0", "0", trade.FeeAsset, trade.TakerSide, trade.TimestampMs,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.SaveTrade(context.Background(), trade); err != nil {
		t.Fatalf("save trade: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
