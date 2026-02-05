package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDBOrderLoader_ListActiveSymbols(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	loader := NewDBOrderLoader(db)

	query := regexp.QuoteMeta(`
		SELECT DISTINCT symbol
		FROM exchange_order.orders
		WHERE status IN (1, 2) AND type = 1
		ORDER BY symbol ASC
	`)

	rows := sqlmock.NewRows([]string{"symbol"}).
		AddRow("BTCUSDT").
		AddRow("ETHUSDT")
	mock.ExpectQuery(query).WillReturnRows(rows)

	got, err := loader.ListActiveSymbols(context.Background())
	if err != nil {
		t.Fatalf("ListActiveSymbols: %v", err)
	}
	if len(got) != 2 || got[0] != "BTCUSDT" || got[1] != "ETHUSDT" {
		t.Fatalf("unexpected symbols: %#v", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestDBOrderLoader_LoadOpenOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	loader := NewDBOrderLoader(db)

	query := regexp.QuoteMeta(`
		SELECT
			o.order_id,
			o.client_order_id,
			o.user_id,
			o.symbol,
			o.side,
			o.type,
			o.time_in_force,
			o.price::text,
			o.orig_qty::text,
			o.executed_qty::text,
			o.create_time_ms,
			sc.price_precision,
			sc.qty_precision
		FROM exchange_order.orders o
		JOIN exchange_order.symbol_configs sc ON sc.symbol = o.symbol
		WHERE o.symbol = $1 AND o.status IN (1, 2) AND o.type = 1
		ORDER BY o.create_time_ms ASC, o.order_id ASC
	`)

	rows := sqlmock.NewRows([]string{
		"order_id",
		"client_order_id",
		"user_id",
		"symbol",
		"side",
		"type",
		"time_in_force",
		"price",
		"orig_qty",
		"executed_qty",
		"create_time_ms",
		"price_precision",
		"qty_precision",
	}).
		AddRow(
			int64(1001),
			"c1",
			int64(42),
			"BTCUSDT",
			SideBuy,
			TypeLimit,
			1,
			"30000.12",
			"0.5",
			"0.1",
			int64(1700000000123),
			2,
			3,
		)

	mock.ExpectQuery(query).WithArgs("BTCUSDT").WillReturnRows(rows)

	got, err := loader.LoadOpenOrders(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LoadOpenOrders: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 order, got %d", len(got))
	}
	if got[0].OrderID != 1001 || got[0].Side != "BUY" || got[0].OrderType != "LIMIT" || got[0].TimeInForce != "GTC" {
		t.Fatalf("unexpected order: %#v", got[0])
	}
	// price_precision=2 => 30000.12 -> 3000012
	if got[0].Price != 3000012 {
		t.Fatalf("expected Price=3000012, got %d", got[0].Price)
	}
	// qty_precision=3 => 0.5 -> 500, 0.1 -> 100, leaves=400
	if got[0].LeavesQty != 400 {
		t.Fatalf("expected LeavesQty=400, got %d", got[0].LeavesQty)
	}
	if got[0].CreatedAt != 1700000000123*1_000_000 {
		t.Fatalf("expected CreatedAt=%d, got %d", 1700000000123*1_000_000, got[0].CreatedAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
