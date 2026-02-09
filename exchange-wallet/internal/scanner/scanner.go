package scanner

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Network is the chain identifier for scanner extensions (e.g. ETH/TRX).
type Network string

const (
	NetworkETH Network = "ETH"
	NetworkTRX Network = "TRX"
)

// Scanner scans blocks in height order and returns where to continue next.
// Implementations should be idempotent: scanning the same height twice is allowed.
type Scanner interface {
	Network() Network
	Confirmations() int64
	Scan(ctx context.Context, fromHeight int64) (ScanResult, error)
}

// ScanResult is a single scan tick result.
type ScanResult struct {
	FromHeight    int64
	ToHeight      int64 // last scanned height (inclusive); 0 if nothing scanned
	NextHeight    int64 // next height to scan
	LatestHeight  int64 // latest chain head observed
	HandledBlocks int64
}

// BaseScanner provides common block scan logic: retry + confirmation window.
// Extend by providing LatestHeightFn and ScanBlockFn.
type BaseScanner struct {
	Net            Network
	Confirms       int64
	MaxRetries     int
	RetryDelay     time.Duration
	LatestHeightFn func(ctx context.Context) (int64, error)
	ScanBlockFn    func(ctx context.Context, height int64) error
}

func (s *BaseScanner) Network() Network { return s.Net }

func (s *BaseScanner) Confirmations() int64 {
	if s.Confirms <= 0 {
		return 1
	}
	return s.Confirms
}

func (s *BaseScanner) Scan(ctx context.Context, fromHeight int64) (ScanResult, error) {
	if s.LatestHeightFn == nil || s.ScanBlockFn == nil {
		return ScanResult{}, errors.New("scanner: missing LatestHeightFn/ScanBlockFn")
	}
	if fromHeight <= 0 {
		fromHeight = 1
	}
	var head int64
	err := s.retry(ctx, "latest height", func() error {
		var e error
		head, e = s.LatestHeightFn(ctx)
		return e
	})
	if err != nil {
		return ScanResult{}, err
	}

	// Only scan blocks with enough confirmations: [fromHeight..head-confirms+1].
	to := head - s.Confirmations() + 1
	if to < fromHeight {
		return ScanResult{FromHeight: fromHeight, NextHeight: fromHeight, LatestHeight: head}, nil
	}

	var handled int64
	for h := fromHeight; h <= to; h++ {
		height := h
		if err := s.retry(ctx, fmt.Sprintf("scan block %d", height), func() error {
			return s.ScanBlockFn(ctx, height)
		}); err != nil {
			return ScanResult{}, err
		}
		handled++
	}

	return ScanResult{
		FromHeight:    fromHeight,
		ToHeight:      to,
		NextHeight:    to + 1,
		LatestHeight:  head,
		HandledBlocks: handled,
	}, nil
}

func (s *BaseScanner) retry(ctx context.Context, op string, fn func() error) error {
	retries := s.MaxRetries
	if retries <= 0 {
		retries = 3
	}
	delay := s.RetryDelay
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}

	var last error
	for i := 0; i < retries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err == nil {
			return nil
		} else {
			last = err
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	return fmt.Errorf("scanner: %s failed after %d retries: %w", op, retries, last)
}
