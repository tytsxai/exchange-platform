package main

import (
	"math"
	"testing"
)

func TestComputeQuoteQty(t *testing.T) {
	// BTCUSDT: price precision=6, qty precision=8
	// price=30000 => 30000 * 1e6
	// qty=0.1 => 0.1 * 1e8
	price := int64(30000_000000)
	qty := int64(10_000000)

	got, err := computeQuoteQty(price, qty, 8)
	if err != nil {
		t.Fatalf("compute quote qty error: %v", err)
	}

	// expected: 3000 USDT => 3000 * 1e6
	const want = int64(3000_000000)
	if got != want {
		t.Fatalf("quote qty = %d, want %d", got, want)
	}
}

func TestComputeQuoteQtyOverflow(t *testing.T) {
	_, err := computeQuoteQty(math.MaxInt64, math.MaxInt64, 0)
	if err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestPow10Int64Validation(t *testing.T) {
	if _, err := pow10Int64(-1); err == nil {
		t.Fatal("expected negative precision error")
	}
	if _, err := pow10Int64(19); err == nil {
		t.Fatal("expected precision too large error")
	}
	v, err := pow10Int64(8)
	if err != nil {
		t.Fatalf("pow10 error: %v", err)
	}
	if v != 100000000 {
		t.Fatalf("pow10(8) = %d, want 100000000", v)
	}
}
