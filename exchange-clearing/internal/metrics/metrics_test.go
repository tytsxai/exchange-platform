package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsCountersAndHistogram(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.ObserveSettlementLatency(1500 * time.Millisecond)
	if err := m.IncBalanceOperation(BalanceOpFreeze); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m.IncLedgerEntries()
	m.IncReconciliationErrors()

	if got := testutil.ToFloat64(m.BalanceOperations.WithLabelValues(BalanceOpFreeze)); got != 1 {
		t.Fatalf("expected balance operation counter 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.LedgerEntries); got != 1 {
		t.Fatalf("expected ledger entries counter 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.ReconciliationErrors); got != 1 {
		t.Fatalf("expected reconciliation errors counter 1, got %v", got)
	}
	if got := testutil.CollectAndCount(m.SettlementLatency); got != 1 {
		t.Fatalf("expected settlement latency histogram collect count 1, got %v", got)
	}
}

func TestIncBalanceOperationInvalid(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	if err := m.IncBalanceOperation("invalid"); err == nil {
		t.Fatal("expected error for invalid balance operation type")
	}
	if got := testutil.CollectAndCount(m.BalanceOperations); got != 0 {
		t.Fatalf("expected balance operations collector count 0, got %v", got)
	}
}

func TestMetricsHandler(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.IncLedgerEntries()
	m.IncReconciliationErrors()
	if err := m.IncBalanceOperation(BalanceOpSettle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ledger_entries_total") {
		t.Fatalf("expected ledger_entries_total in response")
	}
	if !strings.Contains(body, "reconciliation_errors_total") {
		t.Fatalf("expected reconciliation_errors_total in response")
	}
	if !strings.Contains(body, "balance_operations_total") {
		t.Fatalf("expected balance_operations_total in response")
	}
}

func TestNewDefault(t *testing.T) {
	m := NewDefault()
	if err := m.IncBalanceOperation(BalanceOpUnfreeze); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := testutil.ToFloat64(m.BalanceOperations.WithLabelValues(BalanceOpUnfreeze)); got != 1 {
		t.Fatalf("expected balance operation counter 1, got %v", got)
	}
	if m.Handler() == nil {
		t.Fatal("expected handler")
	}
}
