package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/robfig/cron/v3"
)

const (
	availableReconciliationQuery = `
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
`
	frozenReconciliationQuery = `
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
`
	accountBalanceCountQuery = `
SELECT COUNT(DISTINCT user_id), COUNT(DISTINCT asset)
FROM exchange_clearing.account_balances;
`
)

type reconciliationConfig struct {
	DBURL           string
	Verbose         bool
	Alert           bool
	WebhookURL      string
	SlackWebhookURL string
	DingTalkWebhook string
	Fix             bool
	FixThreshold    string
	ReportPath      string
	Cron            string
	StoreHistory    bool
}

type discrepancy struct {
	UserID    int64  `json:"user_id"`
	Asset     string `json:"asset"`
	Kind      string `json:"kind"`
	Diff      string `json:"diff"`
	LedgerSum string `json:"ledger_sum"`
	Balance   string `json:"balance"`
}

var (
	runCLIFunc = runCLI
	exitFunc   = os.Exit
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code := runCLIFunc(ctx, os.Args[1:], os.Stdout, os.Stderr, func(dsn string) (*sql.DB, error) {
		return sql.Open("postgres", dsn)
	})
	exitFunc(code)
}

func parseFlags(args []string) (reconciliationConfig, error) {
	fs := flag.NewFlagSet("reconciliation", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg reconciliationConfig
	fs.StringVar(&cfg.DBURL, "db-url", "", "PostgreSQL connection string")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "show detailed progress")
	fs.BoolVar(&cfg.Alert, "alert", true, "return non-zero exit code on discrepancy")
	fs.StringVar(&cfg.WebhookURL, "webhook-url", "", "webhook url for discrepancy alerts")
	fs.StringVar(&cfg.SlackWebhookURL, "slack-webhook-url", "", "slack webhook url for discrepancy alerts")
	fs.StringVar(&cfg.DingTalkWebhook, "dingtalk-webhook-url", "", "dingtalk webhook url for discrepancy alerts")
	fs.BoolVar(&cfg.Fix, "fix", false, "auto fix small discrepancies")
	fs.StringVar(&cfg.FixThreshold, "fix-threshold", "0.01", "threshold for auto fix differences")
	fs.StringVar(&cfg.ReportPath, "report", "", "write detailed report to file")
	fs.StringVar(&cfg.Cron, "cron", "", "cron expression for scheduled reconciliation runs")
	fs.BoolVar(&cfg.StoreHistory, "history", false, "store reconciliation history in database")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(cfg.DBURL) == "" {
		return cfg, errors.New("missing required --db-url")
	}
	return cfg, nil
}

func runCLI(ctx context.Context, args []string, out, errOut io.Writer, opener func(string) (*sql.DB, error)) int {
	cfg, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	if strings.TrimSpace(cfg.Cron) != "" {
		return runScheduled(ctx, cfg, out, errOut, opener)
	}

	return runOnce(ctx, cfg, out, errOut, opener)
}

func runOnce(ctx context.Context, cfg reconciliationConfig, out, errOut io.Writer, opener func(string) (*sql.DB, error)) int {
	db, err := opener(cfg.DBURL)
	if err != nil {
		fmt.Fprintf(errOut, "failed to connect to database: %v\n", err)
		return 2
	}
	defer db.Close()

	dbPingCtx, dbPingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dbPingCancel()
	if err := db.PingContext(dbPingCtx); err != nil {
		fmt.Fprintf(errOut, "failed to ping database: %v\n", err)
		return 2
	}

	code, err := runWithDB(ctx, db, cfg, out, errOut)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		if code == 0 {
			code = 2
		}
	}
	return code
}

func runScheduled(ctx context.Context, cfg reconciliationConfig, out, errOut io.Writer, opener func(string) (*sql.DB, error)) int {
	if cfg.Verbose {
		fmt.Fprintln(out, "Starting scheduled reconciliation...")
	}

	scheduledCfg := cfg
	scheduledCfg.Alert = false

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cfg.Cron)
	if err != nil {
		fmt.Fprintf(errOut, "invalid cron expression: %v\n", err)
		return 2
	}

	if code := runOnce(ctx, scheduledCfg, out, errOut, opener); code == 2 {
		return code
	}

	c := cron.New(cron.WithParser(parser))
	c.Schedule(schedule, cron.FuncJob(func() {
		if ctx.Err() != nil {
			return
		}
		if cfg.Verbose {
			fmt.Fprintln(out, "Running scheduled reconciliation...")
		}
		if code := runOnce(ctx, scheduledCfg, out, errOut, opener); code != 0 {
			fmt.Fprintf(errOut, "scheduled reconciliation exited with code %d\n", code)
		}
	}))

	c.Start()
	<-ctx.Done()
	c.Stop()
	return 0
}

func runWithDB(ctx context.Context, db *sql.DB, cfg reconciliationConfig, out, errOut io.Writer) (int, error) {
	if cfg.Verbose {
		fmt.Fprintln(out, "Starting reconciliation checks...")
	}

	fixThreshold, err := parseDecimal(cfg.FixThreshold)
	if err != nil {
		return 2, fmt.Errorf("invalid fix threshold: %w", err)
	}

	userCount, assetCount, err := fetchCounts(ctx, db)
	if err != nil {
		return 2, fmt.Errorf("failed to count balances: %w", err)
	}

	if cfg.Verbose {
		fmt.Fprintln(out, "Checking available balances...")
	}
	availableDiscrepancies, err := fetchDiscrepancies(ctx, db, availableReconciliationQuery, "available")
	if err != nil {
		return 2, fmt.Errorf("failed to query available discrepancies: %w", err)
	}

	if cfg.Verbose {
		fmt.Fprintln(out, "Checking frozen balances...")
	}
	frozenDiscrepancies, err := fetchDiscrepancies(ctx, db, frozenReconciliationQuery, "frozen")
	if err != nil {
		return 2, fmt.Errorf("failed to query frozen discrepancies: %w", err)
	}

	discrepancies := append(availableDiscrepancies, frozenDiscrepancies...)
	fixResults := []discrepancy{}
	unresolved := discrepancies
	if cfg.Fix && len(discrepancies) > 0 {
		fixResults, unresolved, err = fixSmallDiscrepancies(ctx, db, discrepancies, fixThreshold)
		if err != nil {
			return 2, fmt.Errorf("failed to fix discrepancies: %w", err)
		}
	}

	report := buildReport(userCount, assetCount, discrepancies, fixResults, unresolved)
	if cfg.ReportPath != "" {
		if err := writeReport(cfg.ReportPath, report); err != nil {
			return 2, fmt.Errorf("failed to write report: %w", err)
		}
	}
	if cfg.StoreHistory {
		if err := storeHistory(ctx, db, report); err != nil {
			return 2, fmt.Errorf("failed to store history: %w", err)
		}
	}

	if len(unresolved) == 0 {
		fmt.Fprintf(out, "✓ Reconciliation passed: %d users, %d assets checked\n", userCount, assetCount)
		return 0, nil
	}

	for _, d := range unresolved {
		fmt.Fprintf(errOut, "✗ Discrepancy found: user_id=%d, asset=%s, type=%s, diff=%s\n", d.UserID, d.Asset, d.Kind, d.Diff)
	}

	if cfg.WebhookURL != "" {
		if err := sendWebhook(ctx, cfg.WebhookURL, unresolved); err != nil {
			fmt.Fprintf(errOut, "webhook alert failed: %v\n", err)
		}
	}
	if cfg.SlackWebhookURL != "" {
		if err := sendSlackWebhook(ctx, cfg.SlackWebhookURL, unresolved); err != nil {
			fmt.Fprintf(errOut, "slack webhook alert failed: %v\n", err)
		}
	}
	if cfg.DingTalkWebhook != "" {
		if err := sendDingTalkWebhook(ctx, cfg.DingTalkWebhook, unresolved); err != nil {
			fmt.Fprintf(errOut, "dingtalk webhook alert failed: %v\n", err)
		}
	}

	if cfg.Alert {
		return 1, nil
	}
	return 0, nil
}

func fetchCounts(ctx context.Context, db *sql.DB) (int64, int64, error) {
	var userCount, assetCount int64
	if err := db.QueryRowContext(ctx, accountBalanceCountQuery).Scan(&userCount, &assetCount); err != nil {
		return 0, 0, err
	}
	return userCount, assetCount, nil
}

func fetchDiscrepancies(ctx context.Context, db *sql.DB, query, kind string) ([]discrepancy, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []discrepancy
	for rows.Next() {
		var userID int64
		var asset string
		var ledgerSum, balance, diff sql.NullString
		if err := rows.Scan(&userID, &asset, &ledgerSum, &balance, &diff); err != nil {
			return nil, err
		}
		diffValue := diff.String
		if !diff.Valid {
			diffValue = ""
		}
		results = append(results, discrepancy{
			UserID:    userID,
			Asset:     asset,
			Kind:      kind,
			Diff:      diffValue,
			LedgerSum: ledgerSum.String,
			Balance:   balance.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func sendWebhook(ctx context.Context, url string, discrepancies []discrepancy) error {
	payload := map[string]interface{}{
		"message":       "reconciliation discrepancies detected",
		"discrepancies": discrepancies,
	}
	return postJSON(ctx, url, payload)
}

func sendSlackWebhook(ctx context.Context, url string, discrepancies []discrepancy) error {
	payload := map[string]string{
		"text": buildAlertMessage("Reconciliation discrepancies detected", discrepancies),
	}
	return postJSON(ctx, url, payload)
}

func sendDingTalkWebhook(ctx context.Context, url string, discrepancies []discrepancy) error {
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": buildAlertMessage("Reconciliation discrepancies detected", discrepancies),
		},
	}
	return postJSON(ctx, url, payload)
}

func postJSON(ctx context.Context, url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %s", resp.Status)
	}
	return nil
}

func buildAlertMessage(title string, discrepancies []discrepancy) string {
	var b strings.Builder
	fmt.Fprintln(&b, title)
	for _, d := range discrepancies {
		fmt.Fprintf(&b, "user_id=%d asset=%s type=%s diff=%s\n", d.UserID, d.Asset, d.Kind, d.Diff)
	}
	return strings.TrimSpace(b.String())
}

func parseDecimal(value string) (*big.Rat, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return new(big.Rat), nil
	}
	r := new(big.Rat)
	if _, ok := r.SetString(trimmed); !ok {
		return nil, fmt.Errorf("invalid decimal: %s", value)
	}
	return r, nil
}

func absRat(value *big.Rat) *big.Rat {
	if value.Sign() < 0 {
		return new(big.Rat).Neg(value)
	}
	return new(big.Rat).Set(value)
}

func fixSmallDiscrepancies(ctx context.Context, db *sql.DB, discrepancies []discrepancy, threshold *big.Rat) ([]discrepancy, []discrepancy, error) {
	var fixed []discrepancy
	var unresolved []discrepancy

	for _, d := range discrepancies {
		if d.Diff == "" || d.LedgerSum == "" {
			unresolved = append(unresolved, d)
			continue
		}
		diffDecimal, err := parseDecimal(d.Diff)
		if err != nil {
			unresolved = append(unresolved, d)
			continue
		}
		if absRat(diffDecimal).Cmp(threshold) == 1 {
			unresolved = append(unresolved, d)
			continue
		}

		var query string
		switch d.Kind {
		case "available":
			query = "UPDATE exchange_clearing.account_balances SET available = $1 WHERE user_id = $2 AND asset = $3"
		case "frozen":
			query = "UPDATE exchange_clearing.account_balances SET frozen = $1 WHERE user_id = $2 AND asset = $3"
		default:
			unresolved = append(unresolved, d)
			continue
		}

		if _, err := db.ExecContext(ctx, query, d.LedgerSum, d.UserID, d.Asset); err != nil {
			return nil, nil, err
		}
		fixed = append(fixed, d)
	}

	return fixed, unresolved, nil
}

type reconciliationReport struct {
	RunAt            string        `json:"run_at"`
	UserCount        int64         `json:"user_count"`
	AssetCount       int64         `json:"asset_count"`
	DiscrepancyCount int           `json:"discrepancy_count"`
	FixedCount       int           `json:"fixed_count"`
	UnresolvedCount  int           `json:"unresolved_count"`
	Discrepancies    []discrepancy `json:"discrepancies"`
	Fixed            []discrepancy `json:"fixed"`
}

func buildReport(userCount, assetCount int64, discrepancies, fixed, unresolved []discrepancy) reconciliationReport {
	return reconciliationReport{
		RunAt:            time.Now().UTC().Format(time.RFC3339),
		UserCount:        userCount,
		AssetCount:       assetCount,
		DiscrepancyCount: len(discrepancies),
		FixedCount:       len(fixed),
		UnresolvedCount:  len(unresolved),
		Discrepancies:    discrepancies,
		Fixed:            fixed,
	}
}

func writeReport(path string, report reconciliationReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func storeHistory(ctx context.Context, db *sql.DB, report reconciliationReport) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS exchange_clearing.reconciliation_history (
    id BIGSERIAL PRIMARY KEY,
    run_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    report JSONB NOT NULL
);`)
	if err != nil {
		return err
	}
	status := "ok"
	if report.UnresolvedCount > 0 {
		status = "discrepancy"
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO exchange_clearing.reconciliation_history (run_at, status, report)
VALUES ($1, $2, $3);`, report.RunAt, status, payload)
	return err
}
