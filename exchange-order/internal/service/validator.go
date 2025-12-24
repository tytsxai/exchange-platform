// Package service price validator
package service

import (
	"context"
	"errors"

	commondecimal "github.com/exchange/common/pkg/decimal"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/repository"
)

var errPriceOutOfRange = errors.New("PRICE_OUT_OF_RANGE")

// SymbolConfigStore 交易对配置读取接口
type SymbolConfigStore interface {
	GetSymbolConfig(ctx context.Context, symbol string) (*repository.SymbolConfig, error)
}

// PriceValidatorConfig 价格保护配置
type PriceValidatorConfig struct {
	Enabled          bool
	DefaultLimitRate commondecimal.Decimal
}

// MatchingPriceClient 行情获取接口
type MatchingPriceClient interface {
	GetLastPrice(symbol string) (int64, error)
}

// PriceValidator 价格保护校验
type PriceValidator struct {
	store    SymbolConfigStore
	matching MatchingPriceClient
	cfg      PriceValidatorConfig
}

// NewPriceValidator 创建价格保护校验器
func NewPriceValidator(store SymbolConfigStore, matching MatchingPriceClient, cfg PriceValidatorConfig) *PriceValidator {
	return &PriceValidator{
		store:    store,
		matching: matching,
		cfg:      cfg,
	}
}

// ReferencePrice 获取参考价
func (v *PriceValidator) ReferencePrice(symbol string) (int64, error) {
	if v == nil || v.matching == nil {
		return 0, errors.New("no reference price")
	}
	return v.matching.GetLastPrice(symbol)
}

// ValidatePrice 校验限价单价格偏离
func (v *PriceValidator) ValidatePrice(symbol, side string, price int64) error {
	if !v.cfg.Enabled {
		return nil
	}

	cfg, err := v.store.GetSymbolConfig(context.Background(), symbol)
	if err != nil {
		return err
	}

	limitRate := v.cfg.DefaultLimitRate
	if cfg != nil && cfg.PriceLimitRate != "" {
		if rate, err := commondecimal.New(cfg.PriceLimitRate); err == nil && rate.Cmp(commondecimal.Zero) > 0 {
			limitRate = *rate
		}
	}

	refValue, err := v.matching.GetLastPrice(symbol)
	if err != nil {
		if errors.Is(err, client.ErrNoReferencePrice) {
			// 当撮合簿为空时允许首单进入（由首单形成参考价）
			return nil
		}
		return err
	}
	if refValue == 0 {
		return nil
	}
	pricePrecision := 0
	if cfg != nil {
		pricePrecision = cfg.PricePrecision
	}
	scale := normalizePrecision(pricePrecision)
	deviationScale := scale
	if deviationScale < defaultPrecision {
		deviationScale = defaultPrecision
	}

	refPrice := commondecimal.FromIntWithScale(refValue, scale)
	priceDec := commondecimal.FromIntWithScale(price, scale)

	deviation := priceDec.Sub(refPrice).Abs().Div(refPrice, deviationScale)
	if deviation.Cmp(&limitRate) > 0 {
		return errPriceOutOfRange
	}

	one := commondecimal.FromInt(1)
	upper := refPrice.Mul(one.Add(&limitRate))
	lower := refPrice.Mul(one.Sub(&limitRate))

	switch side {
	case "BUY":
		if priceDec.Cmp(upper) > 0 {
			return errPriceOutOfRange
		}
	case "SELL":
		if priceDec.Cmp(lower) < 0 {
			return errPriceOutOfRange
		}
	default:
		return errors.New("INVALID_SIDE")
	}

	return nil
}

var _ MatchingPriceClient = (*client.MatchingClient)(nil)
