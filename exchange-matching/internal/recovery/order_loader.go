package recovery

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	commondecimal "github.com/exchange/common/pkg/decimal"
	"github.com/exchange/matching/internal/types"
)

// DBOrderLoader 从订单库加载 open 订单，用于 matching 启动恢复。
type DBOrderLoader struct {
	db *sql.DB
}

func NewDBOrderLoader(db *sql.DB) *DBOrderLoader {
	return &DBOrderLoader{db: db}
}

func (l *DBOrderLoader) ListActiveSymbols(ctx context.Context) ([]string, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("db not configured")
	}
	const query = `
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
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		if strings.TrimSpace(symbol) != "" {
			symbols = append(symbols, symbol)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate symbols: %w", err)
	}
	return symbols, nil
}

func (l *DBOrderLoader) LoadOpenOrders(ctx context.Context, symbol string) ([]*types.OpenOrder, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("db not configured")
	}
	const query = `
		SELECT
			o.order_id,
			COALESCE(o.client_order_id, ''),
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
		WHERE o.symbol = $1
		  AND o.status IN (1, 2)
		  AND o.type = 1
		ORDER BY o.create_time_ms ASC, o.order_id ASC
	`
	rows, err := l.db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("load open orders: %w", err)
	}
	defer rows.Close()

	var orders []*types.OpenOrder
	for rows.Next() {
		var (
			orderID       int64
			clientOrderID string
			userID        int64
			dbSymbol      string
			side          int
			orderType     int
			timeInForce   int
			priceRaw      string
			origQtyRaw    string
			executedRaw   string
			createTimeMs  int64
			pricePrec     int
			qtyPrec       int
		)
		if err := rows.Scan(
			&orderID,
			&clientOrderID,
			&userID,
			&dbSymbol,
			&side,
			&orderType,
			&timeInForce,
			&priceRaw,
			&origQtyRaw,
			&executedRaw,
			&createTimeMs,
			&pricePrec,
			&qtyPrec,
		); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}

		price, err := parseScaledInt(priceRaw, pricePrec)
		if err != nil {
			return nil, fmt.Errorf("parse price: orderID=%d: %w", orderID, err)
		}
		origQty, err := parseScaledInt(origQtyRaw, qtyPrec)
		if err != nil {
			return nil, fmt.Errorf("parse orig_qty: orderID=%d: %w", orderID, err)
		}
		executedQty, err := parseScaledInt(executedRaw, qtyPrec)
		if err != nil {
			return nil, fmt.Errorf("parse executed_qty: orderID=%d: %w", orderID, err)
		}
		leavesQty := origQty - executedQty
		if leavesQty < 0 {
			leavesQty = 0
		}

		orders = append(orders, &types.OpenOrder{
			OrderID:       orderID,
			ClientOrderID: clientOrderID,
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
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	return orders, nil
}

func parseScaledInt(value string, precision int) (int64, error) {
	value = strings.TrimSpace(value)
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

func sideToString(side int) string {
	switch side {
	case 1:
		return "BUY"
	case 2:
		return "SELL"
	default:
		return ""
	}
}

func orderTypeToString(orderType int) string {
	switch orderType {
	case 1:
		return "LIMIT"
	case 2:
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
