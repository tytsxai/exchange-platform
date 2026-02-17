package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"sync"
)

type symbolMeta struct {
	BaseAsset    string
	QuoteAsset   string
	QtyPrecision int
}

type symbolMetaResolver interface {
	Resolve(ctx context.Context, symbol string) (*symbolMeta, error)
}

type dbSymbolMetaResolver struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]*symbolMeta
}

func newDBSymbolMetaResolver(db *sql.DB) *dbSymbolMetaResolver {
	return &dbSymbolMetaResolver{
		db:    db,
		cache: make(map[string]*symbolMeta),
	}
}

func (r *dbSymbolMetaResolver) Resolve(ctx context.Context, symbol string) (*symbolMeta, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("symbol meta resolver not configured")
	}

	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol required")
	}

	r.mu.RLock()
	if cached, ok := r.cache[symbol]; ok && cached != nil {
		clone := *cached
		r.mu.RUnlock()
		return &clone, nil
	}
	r.mu.RUnlock()

	const query = `
		SELECT base_asset, quote_asset, qty_precision
		FROM exchange_order.symbol_configs
		WHERE symbol = $1
	`
	var meta symbolMeta
	if err := r.db.QueryRowContext(ctx, query, symbol).Scan(&meta.BaseAsset, &meta.QuoteAsset, &meta.QtyPrecision); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("symbol config not found: %s", symbol)
		}
		return nil, fmt.Errorf("query symbol config: %w", err)
	}

	if strings.TrimSpace(meta.BaseAsset) == "" || strings.TrimSpace(meta.QuoteAsset) == "" {
		return nil, fmt.Errorf("invalid symbol config: empty asset")
	}
	if _, err := pow10Int64(meta.QtyPrecision); err != nil {
		return nil, fmt.Errorf("invalid qty precision: %w", err)
	}

	r.mu.Lock()
	copyMeta := meta
	r.cache[symbol] = &copyMeta
	r.mu.Unlock()

	return &meta, nil
}

func computeQuoteQty(price, qty int64, qtyPrecision int) (int64, error) {
	if price < 0 || qty < 0 {
		return 0, fmt.Errorf("price/qty must be non-negative")
	}
	scale, err := pow10Int64(qtyPrecision)
	if err != nil {
		return 0, err
	}
	if scale <= 0 {
		return 0, fmt.Errorf("invalid scale")
	}

	product := new(big.Int).Mul(big.NewInt(price), big.NewInt(qty))
	quote := new(big.Int).Quo(product, big.NewInt(scale))
	if !quote.IsInt64() {
		return 0, fmt.Errorf("quote amount overflow")
	}
	return quote.Int64(), nil
}

func pow10Int64(precision int) (int64, error) {
	if precision < 0 || precision > 18 {
		return 0, fmt.Errorf("precision out of range: %d", precision)
	}
	scale := int64(1)
	for i := 0; i < precision; i++ {
		scale *= 10
	}
	return scale, nil
}
