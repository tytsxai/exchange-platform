package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	commondecimal "github.com/exchange/common/pkg/decimal"
)

// OrderLoader 订单加载器接口（用于 matching 启动时恢复订单簿）
//
// 注意：该接口定义对齐 exchange-matching/internal/handler/handler.go。
type OrderLoader interface {
	LoadOpenOrders(ctx context.Context, symbol string) ([]*OpenOrder, error)
	ListActiveSymbols(ctx context.Context) ([]string, error)
}

// OpenOrder 启动恢复用的挂单快照（来自数据库）
//
// 注意：该结构体字段对齐 exchange-matching/internal/types/open_order.go。
type OpenOrder struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	Symbol        string
	Side          string // BUY/SELL
	OrderType     string // LIMIT/MARKET
	TimeInForce   string // GTC/IOC/FOK/POST_ONLY
	Price         int64
	LeavesQty     int64 // 剩余数量 = orig_qty - executed_qty
	CreatedAt     int64 // 纳秒时间戳
}

// DBOrderLoader 使用数据库加载 OPEN 订单（用于恢复订单簿）。
type DBOrderLoader struct {
	db *sql.DB
}

func NewDBOrderLoader(db *sql.DB) *DBOrderLoader {
	return &DBOrderLoader{db: db}
}

func (l *DBOrderLoader) ListActiveSymbols(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT symbol
		FROM exchange_order.orders
		WHERE status IN (1, 2) AND type = 1
		ORDER BY symbol ASC
	`
	rows, err := l.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active symbols: %w", err)
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scan active symbol: %w", err)
		}
		if symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active symbols: %w", err)
	}
	return symbols, nil
}

func (l *DBOrderLoader) LoadOpenOrders(ctx context.Context, symbol string) ([]*OpenOrder, error) {
	// 加载 OPEN 状态（1=NEW, 2=PARTIALLY_FILLED）的 LIMIT 订单（type=1）
	// 同时读取 symbol_configs 的精度用于 DECIMAL -> scaled int64。
	query := `
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
	`
	rows, err := l.db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("load open orders: %w", err)
	}
	defer rows.Close()

	var orders []*OpenOrder
	for rows.Next() {
		var (
			orderID        int64
			clientOrderID  sql.NullString
			userID         int64
			dbSymbol       string
			side           int
			orderType      int
			timeInForce    int
			priceStr       sql.NullString
			origQtyStr     sql.NullString
			executedQtyStr sql.NullString
			createTimeMs   int64
			pricePrecision int
			qtyPrecision   int
		)
		if err := rows.Scan(
			&orderID,
			&clientOrderID,
			&userID,
			&dbSymbol,
			&side,
			&orderType,
			&timeInForce,
			&priceStr,
			&origQtyStr,
			&executedQtyStr,
			&createTimeMs,
			&pricePrecision,
			&qtyPrecision,
		); err != nil {
			return nil, fmt.Errorf("scan open order: %w", err)
		}

		price, err := parseDecimalToScaledInt64(nullStringToString(priceStr), pricePrecision)
		if err != nil {
			return nil, fmt.Errorf("parse price: orderID=%d: %w", orderID, err)
		}
		origQty, err := parseDecimalToScaledInt64(nullStringToString(origQtyStr), qtyPrecision)
		if err != nil {
			return nil, fmt.Errorf("parse orig_qty: orderID=%d: %w", orderID, err)
		}
		executedQty, err := parseDecimalToScaledInt64(nullStringToString(executedQtyStr), qtyPrecision)
		if err != nil {
			return nil, fmt.Errorf("parse executed_qty: orderID=%d: %w", orderID, err)
		}

		leavesQty := origQty - executedQty
		if leavesQty < 0 {
			leavesQty = 0
		}

		orders = append(orders, &OpenOrder{
			OrderID:       orderID,
			ClientOrderID: clientOrderID.String,
			UserID:        userID,
			Symbol:        dbSymbol,
			Side:          sideToString(side),
			OrderType:     orderTypeToString(orderType),
			TimeInForce:   timeInForceToString(timeInForce),
			Price:         price,
			LeavesQty:     leavesQty,
			CreatedAt:     createTimeMs * 1_000_000, // ms -> ns
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open orders: %w", err)
	}

	return orders, nil
}

func parseDecimalToScaledInt64(value string, precision int) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if strings.Contains(value, ".") {
		dec, err := commondecimal.New(value)
		if err != nil {
			return 0, err
		}
		return dec.ToInt(precision), nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func nullStringToString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func sideToString(side int) string {
	switch side {
	case SideBuy:
		return "BUY"
	case SideSell:
		return "SELL"
	default:
		return ""
	}
}

func orderTypeToString(t int) string {
	switch t {
	case TypeLimit:
		return "LIMIT"
	case TypeMarket:
		return "MARKET"
	default:
		return ""
	}
}

func timeInForceToString(tif int) string {
	switch tif {
	case 1:
		return "GTC"
	case 2:
		return "IOC"
	case 3:
		return "FOK"
	case 4:
		return "POST_ONLY"
	default:
		return ""
	}
}
