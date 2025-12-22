package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	BalanceOpFreeze   = "freeze"
	BalanceOpUnfreeze = "unfreeze"
	BalanceOpSettle   = "settle"
)

var balanceOpKinds = map[string]struct{}{
	BalanceOpFreeze:   {},
	BalanceOpUnfreeze: {},
	BalanceOpSettle:   {},
}

// Metrics holds Prometheus metrics for the clearing service.
type Metrics struct {
	SettlementLatency    prometheus.Histogram
	BalanceOperations    *prometheus.CounterVec
	LedgerEntries        prometheus.Counter
	ReconciliationErrors prometheus.Counter
	gatherer             prometheus.Gatherer
}

// NewDefault registers metrics with the default Prometheus registry.
func NewDefault() *Metrics {
	return newMetrics(prometheus.DefaultRegisterer, prometheus.DefaultGatherer)
}

// New registers metrics with the provided registry. If registry is nil, a new
// isolated registry is created.
func New(registry *prometheus.Registry) *Metrics {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}
	return newMetrics(registry, registry)
}

func newMetrics(registerer prometheus.Registerer, gatherer prometheus.Gatherer) *Metrics {
	m := &Metrics{
		SettlementLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "settlement_latency_seconds",
			Help:    "Settlement latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		BalanceOperations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "balance_operations_total",
			Help: "Total balance operations by type.",
		}, []string{"type"}),
		LedgerEntries: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ledger_entries_total",
			Help: "Total ledger entries.",
		}),
		ReconciliationErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconciliation_errors_total",
			Help: "Total reconciliation errors.",
		}),
		gatherer: gatherer,
	}

	registerer.MustRegister(
		m.SettlementLatency,
		m.BalanceOperations,
		m.LedgerEntries,
		m.ReconciliationErrors,
	)

	return m
}

// Handler returns an HTTP handler that exposes metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.gatherer, promhttp.HandlerOpts{})
}

// ObserveSettlementLatency records settlement latency.
func (m *Metrics) ObserveSettlementLatency(d time.Duration) {
	m.SettlementLatency.Observe(d.Seconds())
}

// IncBalanceOperation increments the counter for a balance operation type.
func (m *Metrics) IncBalanceOperation(kind string) error {
	if _, ok := balanceOpKinds[kind]; !ok {
		return fmt.Errorf("unknown balance operation type: %s", kind)
	}
	m.BalanceOperations.WithLabelValues(kind).Inc()
	return nil
}

// IncLedgerEntries increments the ledger entries counter by 1.
func (m *Metrics) IncLedgerEntries() {
	m.LedgerEntries.Inc()
}

// IncReconciliationErrors increments the reconciliation errors counter by 1.
func (m *Metrics) IncReconciliationErrors() {
	m.ReconciliationErrors.Inc()
}
