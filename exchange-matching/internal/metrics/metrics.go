package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry = prometheus.NewRegistry()
	once     sync.Once

	matchingLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "matching_latency_seconds",
		Help:    "Latency of order matching in seconds.",
		Buckets: prometheus.DefBuckets,
	})
	tradesCreated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trades_created_total",
			Help: "Total number of trades created.",
		},
		[]string{"symbol"},
	)
	orderbookDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "orderbook_depth",
			Help: "Current orderbook depth.",
		},
		[]string{"symbol", "side"},
	)
	matchingThroughput = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "matching_throughput",
		Help: "Total number of orders processed by matching.",
	})
)

// Init registers metrics with the registry once.
func Init() {
	once.Do(func() {
		registry.MustRegister(
			prometheus.NewGoCollector(),
			prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
			matchingLatency,
			tradesCreated,
			orderbookDepth,
			matchingThroughput,
		)
	})
}

// Handler exposes the Prometheus metrics endpoint handler.
func Handler() http.Handler {
	Init()
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// ObserveMatchingLatency records a matching latency duration.
func ObserveMatchingLatency(d time.Duration) {
	Init()
	matchingLatency.Observe(d.Seconds())
}

// IncTradesCreated increments the trades created counter for a symbol.
func IncTradesCreated(symbol string) {
	Init()
	tradesCreated.WithLabelValues(symbol).Inc()
}

// SetOrderbookDepth sets the current orderbook depth for a symbol and side.
func SetOrderbookDepth(symbol, side string, depth float64) {
	Init()
	orderbookDepth.WithLabelValues(symbol, side).Set(depth)
}

// AddMatchingThroughput increments the matching throughput counter by n.
func AddMatchingThroughput(n int) {
	Init()
	if n <= 0 {
		return
	}
	matchingThroughput.Add(float64(n))
}
