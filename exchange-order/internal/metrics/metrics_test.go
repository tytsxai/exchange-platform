package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func findMetric(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	return nil
}

func TestMetricsCountersAndGauge(t *testing.T) {
	m := New()

	m.IncOrderCreated("BTCUSDT", "BUY")
	m.IncOrderRejected("INVALID_PRICE")
	m.SetActiveOrders(5)
	m.IncActiveOrders()
	m.DecActiveOrders()
	m.ObserveOrderLatency(250 * time.Millisecond)
	m.ObserveOrderLatencySeconds(0.5)

	families, err := m.registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	created := findMetric(t, families, "order_created_total")
	if created == nil || len(created.GetMetric()) != 1 {
		t.Fatalf("expected order_created_total metric")
	}
	if got := created.GetMetric()[0].GetCounter().GetValue(); got != 1 {
		t.Fatalf("expected order_created_total=1, got %v", got)
	}

	rejected := findMetric(t, families, "order_rejected_total")
	if rejected == nil || len(rejected.GetMetric()) != 1 {
		t.Fatalf("expected order_rejected_total metric")
	}
	if got := rejected.GetMetric()[0].GetCounter().GetValue(); got != 1 {
		t.Fatalf("expected order_rejected_total=1, got %v", got)
	}

	active := findMetric(t, families, "active_orders_count")
	if active == nil || len(active.GetMetric()) != 1 {
		t.Fatalf("expected active_orders_count metric")
	}
	if got := active.GetMetric()[0].GetGauge().GetValue(); got != 5 {
		t.Fatalf("expected active_orders_count=5, got %v", got)
	}

	latency := findMetric(t, families, "order_latency_seconds")
	if latency == nil || len(latency.GetMetric()) != 1 {
		t.Fatalf("expected order_latency_seconds metric")
	}
	if got := latency.GetMetric()[0].GetHistogram().GetSampleCount(); got != 2 {
		t.Fatalf("expected order_latency_seconds count=2, got %v", got)
	}
}

func TestMetricsHandler(t *testing.T) {
	m := New()
	m.IncOrderCreated("BTCUSDT", "BUY")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" || !strings.Contains(body, "order_created_total") {
		t.Fatalf("expected metrics output to include order_created_total")
	}
}
