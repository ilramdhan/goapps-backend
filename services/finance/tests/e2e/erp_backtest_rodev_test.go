package e2e

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

// TestErpBacktest_RODev is the USER-RUN real-data backtest (P5-T10c).
//
// RUNBOOK (user only; sub-agents and CI must NEVER run this against Oracle):
//
//  1. Use ONLY the existing READ-ONLY ALTHARADEV (DEV) user. Never a
//     write-capable, production or staging account.
//
//  2. From goapps-backend/services/finance run (PG comes from a local
//     pgcontainer, Oracle is read through oracle.NewReadOnlyDB only):
//
//     DOCKER_HOST=unix://$HOME/.colima/default/docker.sock \
//     TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
//     TESTCONTAINERS_RYUK_DISABLED=true INTEGRATION_TEST=true \
//     E2E_TEST=true ORACLE_RO_TEST=true \
//     ORACLE_RO_DSN='oracle://<ro_user>:<password>@<dev_host>:<port>/<service>' \
//     go test ./tests/e2e/ -run TestErpBacktest_RODev -count=1 -v -timeout 600s
//
//     Optional: ORACLE_RO_PERIODS=202604,202606 (default 202604,202606,202607,202608)
//     and BACKTEST_OUT_DIR=/path for the CSVs (default: a temp dir, logged).
//
//  3. Paste the full -v output and attach the per-period CSVs to the ledger
//     under P5-T10.
//
// What it does: per period runs the SHADOW backtest (real Oracle demand and
// ADJ readers, legacy-rate reader) on an empty migrated PG, logs the counts
// per class and Failed, persists the report and writes backtest_<period>.csv.
// A step error for one period is logged and the next period still runs.
// Without both env flags it skips and opens no connection.
func TestErpBacktest_RODev(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ORACLE_RO_TEST")), "true") {
		t.Skip("ORACLE_RO_TEST!=true: user-run read-only DEV backtest skipped (P5-T10c)")
	}
	dsn := strings.TrimSpace(os.Getenv("ORACLE_RO_DSN"))
	require.NotEmpty(t, dsn, "ORACLE_RO_TEST=true but ORACLE_RO_DSN is empty; set the READ-ONLY DEV DSN")
	periods := []string{"202604", "202606", "202607", "202608"}
	if env := strings.TrimSpace(os.Getenv("ORACLE_RO_PERIODS")); env != "" {
		periods = nil
		for _, p := range strings.Split(env, ",") {
			if p = strings.TrimSpace(p); p != "" {
				periods = append(periods, p)
			}
		}
	}
	outDir := strings.TrimSpace(os.Getenv("BACKTEST_OUT_DIR"))
	if outDir == "" {
		outDir = t.TempDir()
	}
	t.Logf("RO backtest CSV dir: %s", outDir)

	odb, err := sql.Open("oracle", dsn)
	require.NoError(t, err)
	odb.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = odb.Close() })
	pctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, odb.PingContext(pctx))
	rq := &erpRecordingQuerier{inner: oracle.NewReadOnlyDB(odb)}
	defer func() {
		require.NotEmpty(t, rq.stmts, "no Oracle statements recorded")
		for _, s := range rq.stmts {
			assert.NoError(t, oracle.CheckReadOnly(s), "statement not read-only")
		}
		t.Logf("RO oracle statements: %d (all passed CheckReadOnly)", len(rq.stmts))
	}()

	reader, err := oracle.NewErpDemandReader(rq, "view", "development")
	require.NoError(t, err)
	prober := oracle.NewErpAdjReader(rq)

	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_backtest_rodev", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	db := postgres.NewDBFromSQL(raw)

	st := newErpSteps(db, reader, prober)
	h := erpapp.NewBacktestHandler(st.create, st.load, st.cov, st.derive, st.validate,
		postgres.NewErpStdCostRepository(db), oracle.NewLegacyAdjRateReader(rq))
	reports := postgres.NewErpBacktestReportRepository(db)

	for _, p := range periods {
		report, batchID, runErr := h.Run(ctx, erpapp.BacktestCommand{Period: p, Actor: "backtest-rodev"})
		if runErr != nil {
			t.Errorf("RO backtest period=%s: %v", p, runErr)
			continue
		}
		t.Logf("RO backtest period=%s batch=%d lines=%d counts=%+v failed=%v",
			p, batchID, len(report.Lines), report.Counts, report.Failed)
		if rerr := reports.Replace(ctx, batchID, p, report); rerr != nil {
			t.Errorf("RO backtest period=%s persist: %v", p, rerr)
		}
		path := filepath.Join(outDir, "backtest_"+p+".csv")
		f, ferr := os.Create(path) //nolint:gosec // test output path
		if ferr != nil {
			t.Errorf("RO backtest period=%s csv create: %v", p, ferr)
			continue
		}
		if werr := report.WriteCSV(f, p); werr != nil {
			t.Errorf("RO backtest period=%s csv write: %v", p, werr)
		}
		_ = f.Close()
		t.Logf("RO backtest CSV: %s", path)
	}
}
