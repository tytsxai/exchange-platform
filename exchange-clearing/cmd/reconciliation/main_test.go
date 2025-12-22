package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestParseFlags(t *testing.T) {
	cfg, err := parseFlags([]string{"--db-url", "postgres://localhost/db", "--verbose", "--alert=false", "--fix", "--report", "report.json", "--cron", "*/5 * * * *"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DBURL != "postgres://localhost/db" {
		t.Fatalf("unexpected db url: %s", cfg.DBURL)
	}
	if !cfg.Verbose {
		t.Fatalf("expected verbose true")
	}
	if cfg.Alert {
		t.Fatalf("expected alert false")
	}
	if !cfg.Fix {
		t.Fatalf("expected fix true")
	}
	if cfg.ReportPath != "report.json" {
		t.Fatalf("expected report path set")
	}
	if cfg.Cron != "*/5 * * * *" {
		t.Fatalf("expected cron to be set")
	}

	if _, err := parseFlags([]string{}); err == nil {
		t.Fatalf("expected error for missing db url")
	}
	if _, err := parseFlags([]string{"--db-url"}); err == nil {
		t.Fatalf("expected error for invalid args")
	}
}

func TestQueriesMatchSpec(t *testing.T) {
	expectedAvailable := strings.TrimSpace(`
SELECT
    le.user_id,
    le.asset,
    SUM(le.available_delta) as ledger_available_sum,
    ab.available as balance_available,
    SUM(le.available_delta) - ab.available as available_diff
FROM exchange_clearing.ledger_entries le
JOIN exchange_clearing.account_balances ab
    ON le.user_id = ab.user_id AND le.asset = ab.asset
GROUP BY le.user_id, le.asset, ab.available
HAVING SUM(le.available_delta) != ab.available;
`)
	expectedFrozen := strings.TrimSpace(`
SELECT
    le.user_id,
    le.asset,
    SUM(le.frozen_delta) as ledger_frozen_sum,
    ab.frozen as balance_frozen,
    SUM(le.frozen_delta) - ab.frozen as frozen_diff
FROM exchange_clearing.ledger_entries le
JOIN exchange_clearing.account_balances ab
    ON le.user_id = ab.user_id AND le.asset = ab.asset
GROUP BY le.user_id, le.asset, ab.frozen
HAVING SUM(le.frozen_delta) != ab.frozen;
`)

	if strings.TrimSpace(availableReconciliationQuery) != expectedAvailable {
		t.Fatalf("available query does not match spec")
	}
	if strings.TrimSpace(frozenReconciliationQuery) != expectedFrozen {
		t.Fatalf("frozen query does not match spec")
	}
}

func TestReconcileNoDiscrepancy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(2, 3))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:   "postgres://localhost/db",
		Alert:   true,
		Verbose: false,
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Reconciliation passed") {
		t.Fatalf("expected pass message, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestReconcileDiscrepancy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
			AddRow(123, "BTC", "10.0", "9.0", "1.0"))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL: "postgres://localhost/db",
		Alert: true,
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if errOut.Len() == 0 {
		t.Fatalf("expected stderr output")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunWithDBCountError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnError(errors.New("count failed"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL: "postgres://localhost/db",
		Alert: true,
	}, &out, &errOut)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunWithDBAvailableQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnError(errors.New("available query failed"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL: "postgres://localhost/db",
		Alert: true,
	}, &out, &errOut)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunWithDBFrozenQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnError(errors.New("frozen query failed"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL: "postgres://localhost/db",
		Alert: true,
	}, &out, &errOut)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFetchDiscrepanciesNullDiff(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
			AddRow(1, "BTC", "1", "1", nil))

	results, err := fetchDiscrepancies(context.Background(), db, "SELECT 1", "available")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 1 || results[0].Diff != "" {
		t.Fatalf("expected empty diff value")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFetchDiscrepanciesRowError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
		AddRow(1, "BTC", "1", "1", "0")
	rows.RowError(0, errors.New("row error"))

	mock.ExpectQuery("SELECT 1").WillReturnRows(rows)

	if _, err := fetchDiscrepancies(context.Background(), db, "SELECT 1", "available"); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSendWebhookInvalidURL(t *testing.T) {
	if err := sendWebhook(context.Background(), "http://[::1", []discrepancy{{UserID: 1, Asset: "BTC", Kind: "available", Diff: "1"}}); err == nil {
		t.Fatalf("expected error for invalid url")
	}
}

func TestRunCLIHandlesRunWithDBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnError(errors.New("count failed"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI(context.Background(), []string{"--db-url", "postgres://localhost/db"}, &out, &errOut, func(dsn string) (*sql.DB, error) {
		return db, nil
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "failed to count balances") {
		t.Fatalf("expected count error, got %q", errOut.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunCLIValidationAndOpenErrors(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := runCLI(context.Background(), []string{}, &out, &errOut, func(dsn string) (*sql.DB, error) {
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "missing required --db-url") {
		t.Fatalf("expected missing db url error, got %q", errOut.String())
	}

	errOut.Reset()
	code = runCLI(context.Background(), []string{"--db-url", "postgres://localhost/db"}, &out, &errOut, func(dsn string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "failed to connect to database") {
		t.Fatalf("expected connect error, got %q", errOut.String())
	}
}

func TestRunCLIPingError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectPing().WillReturnError(errors.New("ping failed"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI(context.Background(), []string{"--db-url", "postgres://localhost/db"}, &out, &errOut, func(dsn string) (*sql.DB, error) {
		return db, nil
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "failed to ping database") {
		t.Fatalf("expected ping error, got %q", errOut.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWebhookSuccessAndAlertDisabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
			AddRow(123, "BTC", "10.0", "9.0", "1.0"))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:      "postgres://localhost/db",
		Alert:      false,
		WebhookURL: server.URL,
		Verbose:    true,
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Starting reconciliation checks") {
		t.Fatalf("expected verbose output")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWebhookFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
			AddRow(123, "BTC", "10.0", "9.0", "1.0"))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:      "postgres://localhost/db",
		Alert:      true,
		WebhookURL: server.URL,
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "webhook alert failed") {
		t.Fatalf("expected webhook failure output, got %q", errOut.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFixSmallDiscrepancies(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
			AddRow(123, "BTC", "10.0", "9.995", "0.005"))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))
	mock.ExpectExec("UPDATE exchange_clearing.account_balances SET available").
		WithArgs("10.0", int64(123), "BTC").
		WillReturnResult(sqlmock.NewResult(1, 1))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:        "postgres://localhost/db",
		Alert:        true,
		Fix:          true,
		FixThreshold: "0.01",
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWriteReport(t *testing.T) {
	report := reconciliationReport{
		RunAt:            "2024-01-01T00:00:00Z",
		UserCount:        2,
		AssetCount:       3,
		DiscrepancyCount: 0,
		FixedCount:       0,
		UnresolvedCount:  0,
	}
	path := t.TempDir() + "/report.json"
	if err := writeReport(path, report); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected report file, got %v", err)
	}
	if !strings.Contains(string(data), `"user_count": 2`) {
		t.Fatalf("expected report contents")
	}
}

func TestStoreHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS exchange_clearing.reconciliation_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO exchange_clearing.reconciliation_history").
		WithArgs("2024-01-01T00:00:00Z", "ok", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	report := reconciliationReport{
		RunAt:            "2024-01-01T00:00:00Z",
		UserCount:        1,
		AssetCount:       1,
		DiscrepancyCount: 0,
		FixedCount:       0,
		UnresolvedCount:  0,
	}
	if err := storeHistory(context.Background(), db, report); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSlackAndDingTalkWebhook(t *testing.T) {
	var payloads []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payloads = append(payloads, payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	discrepancies := []discrepancy{{UserID: 1, Asset: "BTC", Kind: "available", Diff: "1"}}
	if err := sendSlackWebhook(context.Background(), server.URL, discrepancies); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := sendDingTalkWebhook(context.Background(), server.URL, discrepancies); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(payloads) != 2 {
		t.Fatalf("expected two payloads")
	}
	if _, ok := payloads[0]["text"]; !ok {
		t.Fatalf("expected slack payload text")
	}
	if payloads[1]["msgtype"] != "text" {
		t.Fatalf("expected dingtalk msgtype text")
	}
}

func TestParseDecimalAndAbsRat(t *testing.T) {
	value, err := parseDecimal("-1.5")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if absRat(value).String() != "3/2" {
		t.Fatalf("expected absolute value 1.5, got %s", absRat(value).String())
	}
	if _, err := parseDecimal("nope"); err == nil {
		t.Fatalf("expected error for invalid decimal")
	}
}

func TestBuildAlertMessage(t *testing.T) {
	msg := buildAlertMessage("Alert", []discrepancy{{UserID: 1, Asset: "BTC", Kind: "available", Diff: "1"}})
	if !strings.Contains(msg, "Alert") || !strings.Contains(msg, "user_id=1") {
		t.Fatalf("expected alert message content")
	}
}

func TestRunScheduledInvalidCron(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runScheduled(context.Background(), reconciliationConfig{
		DBURL: "postgres://localhost/db",
		Cron:  "invalid",
	}, &out, &errOut, func(dsn string) (*sql.DB, error) {
		return nil, errors.New("should not open")
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "invalid cron expression") {
		t.Fatalf("expected cron error, got %q", errOut.String())
	}
}

func TestRunWithDBReportAndHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS exchange_clearing.reconciliation_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO exchange_clearing.reconciliation_history").
		WithArgs(sqlmock.AnyArg(), "ok", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	path := t.TempDir() + "/report.json"
	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:        "postgres://localhost/db",
		Alert:        true,
		ReportPath:   path,
		StoreHistory: true,
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected report file, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunWithDBInvalidFixThreshold(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:        "postgres://localhost/db",
		Alert:        true,
		FixThreshold: "bad",
	}, &out, &errOut)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMainUsesInjectedFunctions(t *testing.T) {
	originalRunCLI := runCLIFunc
	originalExit := exitFunc
	defer func() {
		runCLIFunc = originalRunCLI
		exitFunc = originalExit
	}()

	runCalled := false
	runCLIFunc = func(ctx context.Context, args []string, out, errOut io.Writer, opener func(string) (*sql.DB, error)) int {
		runCalled = true
		return 0
	}

	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	originalArgs := os.Args
	os.Args = []string{"reconciliation"}
	defer func() { os.Args = originalArgs }()

	main()
	if !runCalled {
		t.Fatalf("expected runCLI to be called")
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunScheduledValidCron(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan int, 1)
	go func() {
		code := runScheduled(ctx, reconciliationConfig{
			DBURL: "postgres://localhost/db",
			Cron:  "*/1 * * * *",
		}, &bytes.Buffer{}, &bytes.Buffer{}, func(dsn string) (*sql.DB, error) {
			return db, nil
		})
		done <- code
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	code := <-done
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunScheduledOpenError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runScheduled(context.Background(), reconciliationConfig{
		DBURL: "postgres://localhost/db",
		Cron:  "*/1 * * * *",
	}, &out, &errOut, func(dsn string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "failed to connect to database") {
		t.Fatalf("expected connect error, got %q", errOut.String())
	}
}

func TestFixSmallDiscrepanciesBranches(t *testing.T) {
	threshold, err := parseDecimal("0.01")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	fixed, unresolved, err := fixSmallDiscrepancies(context.Background(), nil, []discrepancy{
		{UserID: 1, Asset: "BTC", Kind: "available", Diff: "0.1", LedgerSum: "10"},
	}, threshold)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fixed) != 0 || len(unresolved) != 1 {
		t.Fatalf("expected unresolved large diff")
	}

	fixed, unresolved, err = fixSmallDiscrepancies(context.Background(), nil, []discrepancy{
		{UserID: 1, Asset: "BTC", Kind: "unknown", Diff: "0.001", LedgerSum: "10"},
	}, threshold)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fixed) != 0 || len(unresolved) != 1 {
		t.Fatalf("expected unresolved unknown kind")
	}

	fixed, unresolved, err = fixSmallDiscrepancies(context.Background(), nil, []discrepancy{
		{UserID: 1, Asset: "BTC", Kind: "available", Diff: "bad", LedgerSum: "10"},
	}, threshold)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fixed) != 0 || len(unresolved) != 1 {
		t.Fatalf("expected unresolved invalid diff")
	}

	fixed, unresolved, err = fixSmallDiscrepancies(context.Background(), nil, []discrepancy{
		{UserID: 1, Asset: "BTC", Kind: "available", Diff: "0.001", LedgerSum: ""},
	}, threshold)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fixed) != 0 || len(unresolved) != 1 {
		t.Fatalf("expected unresolved missing ledger sum")
	}
}

func TestFixSmallDiscrepanciesExecError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE exchange_clearing.account_balances SET available").
		WillReturnError(errors.New("update failed"))

	threshold, err := parseDecimal("0.01")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_, _, err = fixSmallDiscrepancies(context.Background(), db, []discrepancy{
		{UserID: 1, Asset: "BTC", Kind: "available", Diff: "0.001", LedgerSum: "10"},
	}, threshold)
	if err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWriteReportError(t *testing.T) {
	report := reconciliationReport{RunAt: "2024-01-01T00:00:00Z"}
	if err := writeReport(t.TempDir(), report); err == nil {
		t.Fatalf("expected error writing report to directory")
	}
}

func TestStoreHistoryDiscrepancyStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS exchange_clearing.reconciliation_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO exchange_clearing.reconciliation_history").
		WithArgs("2024-01-01T00:00:00Z", "discrepancy", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	report := reconciliationReport{
		RunAt:           "2024-01-01T00:00:00Z",
		UnresolvedCount: 1,
	}
	if err := storeHistory(context.Background(), db, report); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStoreHistoryErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS exchange_clearing.reconciliation_history").
		WillReturnError(errors.New("create failed"))

	report := reconciliationReport{RunAt: "2024-01-01T00:00:00Z"}
	if err := storeHistory(context.Background(), db, report); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db2.Close()

	mock2.ExpectExec("CREATE TABLE IF NOT EXISTS exchange_clearing.reconciliation_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock2.ExpectExec("INSERT INTO exchange_clearing.reconciliation_history").
		WillReturnError(errors.New("insert failed"))

	if err := storeHistory(context.Background(), db2, report); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunWithDBSlackAndDingTalkErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT user_id\\), COUNT\\(DISTINCT asset\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_count", "asset_count"}).AddRow(1, 1))
	mock.ExpectQuery("SUM\\(le.available_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_available_sum", "balance_available", "available_diff"}).
			AddRow(123, "BTC", "10.0", "9.0", "1.0"))
	mock.ExpectQuery("SUM\\(le.frozen_delta\\)").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "asset", "ledger_frozen_sum", "balance_frozen", "frozen_diff"}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	code, err := runWithDB(context.Background(), db, reconciliationConfig{
		DBURL:           "postgres://localhost/db",
		Alert:           true,
		SlackWebhookURL: server.URL,
		DingTalkWebhook: server.URL,
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "slack webhook alert failed") {
		t.Fatalf("expected slack webhook failure")
	}
	if !strings.Contains(errOut.String(), "dingtalk webhook alert failed") {
		t.Fatalf("expected dingtalk webhook failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
