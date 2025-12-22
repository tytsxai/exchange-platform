package service

import (
	"strconv"
	"strings"

	commondecimal "github.com/exchange/common/pkg/decimal"
)

const defaultPrecision = 8

func normalizePrecision(precision int) int {
	if precision <= 0 {
		return defaultPrecision
	}
	return precision
}

func scaleFactor(precision int) int64 {
	precision = normalizePrecision(precision)
	factor := int64(1)
	for i := 0; i < precision; i++ {
		factor *= 10
	}
	return factor
}

func parseScaledValue(value string, precision int) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if strings.Contains(value, ".") {
		dec, err := commondecimal.New(value)
		if err != nil {
			return 0, err
		}
		return dec.ToInt(normalizePrecision(precision)), nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func quoteQty(price, qty int64, qtyPrecision int) int64 {
	return price * qty / scaleFactor(qtyPrecision)
}
