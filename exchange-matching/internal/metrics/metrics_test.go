package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsUpdates(t *testing.T) {
	Init()

	startTrades := testutil.ToFloat64(tradesCreated.WithLabelValues("BTCUSDT"))
	startThroughput := testutil.ToFloat64(matchingThroughput)
	startHistogramCount := getHistogramSampleCount(t)

	ObserveMatchingLatency(25 * time.Millisecond)
	IncTradesCreated("BTCUSDT")
	SetOrderbookDepth("BTCUSDT", "buy", 12)
	AddMatchingThroughput(3)

	if got := testutil.ToFloat64(tradesCreated.WithLabelValues("BTCUSDT")); got != startTrades+1 {
		t.Fatalf("trades_created_total mismatch: got %v want %v", got, startTrades+1)
	}

	if got := testutil.ToFloat64(matchingThroughput); got != startThroughput+3 {
		t.Fatalf("matching_throughput mismatch: got %v want %v", got, startThroughput+3)
	}

	if got := testutil.ToFloat64(orderbookDepth.WithLabelValues("BTCUSDT", "buy")); got != 12 {
		t.Fatalf("orderbook_depth mismatch: got %v want 12", got)
	}

	if got := getHistogramSampleCount(t); got != startHistogramCount+1 {
		t.Fatalf("matching_latency_seconds sample count mismatch: got %v want %v", got, startHistogramCount+1)
	}
}

func TestHandlerRegistersMetrics(t *testing.T) {
	Handler()
	IncTradesCreated("ETHUSDT")
	SetOrderbookDepth("ETHUSDT", "sell", 7)
	AddMatchingThroughput(1)
	ObserveMatchingLatency(10 * time.Millisecond)

	count, err := testutil.GatherAndCount(
		registry,
		"matching_latency_seconds",
		"trades_created_total",
		"orderbook_depth",
		"matching_throughput",
	)
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if count < 4 {
		t.Fatalf("expected metrics to be registered, got count %d", count)
	}
}

func TestAddMatchingThroughputNoop(t *testing.T) {
	start := testutil.ToFloat64(matchingThroughput)
	AddMatchingThroughput(0)
	AddMatchingThroughput(-2)
	if got := testutil.ToFloat64(matchingThroughput); got != start {
		t.Fatalf("matching_throughput changed on non-positive add: got %v want %v", got, start)
	}
}

func getHistogramSampleCount(t *testing.T) uint64 {
	t.Helper()
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather histogram: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "matching_latency_seconds" {
			continue
		}
		metrics := mf.GetMetric()
		if len(metrics) == 0 {
			return 0
		}
		return metrics[0].GetHistogram().GetSampleCount()
	}
	return 0
}
