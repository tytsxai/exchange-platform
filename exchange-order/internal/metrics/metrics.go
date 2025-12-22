package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics wraps Prometheus metrics for the order service.
type Metrics struct {
	registry      *prometheus.Registry
	orderCreated  *prometheus.CounterVec
	orderLatency  prometheus.Histogram
	orderRejected *prometheus.CounterVec
	activeOrders  prometheus.Gauge
}

// New creates a metrics registry and registers order metrics.
func New() *Metrics {
	registry := prometheus.NewRegistry()

	orderCreated := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "order_created_total",
		Help: "Total number of created orders.",
	}, []string{"symbol", "side"})

	orderLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "order_latency_seconds",
		Help:    "Latency for order creation in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	orderRejected := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "order_rejected_total",
		Help: "Total number of rejected orders.",
	}, []string{"reason"})

	activeOrders := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "active_orders_count",
		Help: "Current number of active orders.",
	})

	registry.MustRegister(orderCreated, orderLatency, orderRejected, activeOrders)

	return &Metrics{
		registry:      registry,
		orderCreated:  orderCreated,
		orderLatency:  orderLatency,
		orderRejected: orderRejected,
		activeOrders:  activeOrders,
	}
}

// Handler exposes the metrics registry via HTTP.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// IncOrderCreated increments the created order counter.
func (m *Metrics) IncOrderCreated(symbol, side string) {
	m.orderCreated.WithLabelValues(symbol, side).Inc()
}

// ObserveOrderLatency records order creation latency.
func (m *Metrics) ObserveOrderLatency(d time.Duration) {
	m.orderLatency.Observe(d.Seconds())
}

// ObserveOrderLatencySeconds records order creation latency in seconds.
func (m *Metrics) ObserveOrderLatencySeconds(seconds float64) {
	m.orderLatency.Observe(seconds)
}

// IncOrderRejected increments the rejected order counter.
func (m *Metrics) IncOrderRejected(reason string) {
	m.orderRejected.WithLabelValues(reason).Inc()
}

// SetActiveOrders sets the active orders gauge.
func (m *Metrics) SetActiveOrders(count int) {
	m.activeOrders.Set(float64(count))
}

// IncActiveOrders increments the active orders gauge.
func (m *Metrics) IncActiveOrders() {
	m.activeOrders.Inc()
}

// DecActiveOrders decrements the active orders gauge.
func (m *Metrics) DecActiveOrders() {
	m.activeOrders.Dec()
}
