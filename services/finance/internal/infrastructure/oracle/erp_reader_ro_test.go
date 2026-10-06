package oracle_test

// erp_reader_ro_test.go — P3-T8 USER-RUN read-only DEV test (QG-USER-SQL, AC-04).
//
// RUNBOOK (user only; sub-agents and CI must NEVER run this against Oracle):
//
//  1. Use ONLY the existing READ-ONLY ALTHARADEV (DEV) user (PR-5). Never a
//     write-capable, production or staging account.
//  2. From goapps-backend/services/finance run:
//
//     ORACLE_RO_TEST=true \
//     ORACLE_RO_DSN='oracle://<ro_user>:<password>@<dev_host>:<port>/<service>' \
//     go test ./internal/infrastructure/oracle/ -run RO -count=1 -v
//
//     Optional: ORACLE_RO_PERIOD=YYYYMM (default 202608).
//  3. Paste the full -v output (the "RO counts" log lines) into the ledger
//     under P3-T8.
//
// What it does: opens the DSN, wraps it in oracle.NewReadOnlyDB (every
// statement passes CheckReadOnly; the type has no Exec/BeginTx), then runs
// SELECT-only reads: ADJ demand for the period via demand_source=base_tables
// (appEnv "development"), the ADJ head probe, and the OM_ITEM / OM_GRADE_CODE_1
// masters. It asserts the reads succeed and the counts are > 0. It never
// writes. Without ORACLE_RO_TEST=true it skips and opens no connection.

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

const (
	roTestEnv       = "ORACLE_RO_TEST"
	roDSNEnv        = "ORACLE_RO_DSN"
	roPeriodEnv     = "ORACLE_RO_PERIOD"
	roDefaultPeriod = "202608"
)

// recordingQuerier forwards to the guarded querier and records each statement
// so the test can assert every one was a guarded SELECT.
type recordingQuerier struct {
	inner oracle.ReadOnlyQuerier
	stmts []string
}

func (r *recordingQuerier) QueryRO(ctx context.Context, query string, args ...any) (oracle.Rows, error) {
	r.stmts = append(r.stmts, query)
	return r.inner.QueryRO(ctx, query, args...)
}

func openRODev(t *testing.T) *recordingQuerier {
	t.Helper()
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(roTestEnv)), "true") {
		t.Skipf("%s!=true: user-run read-only DEV test skipped (P3-T8)", roTestEnv)
	}
	dsn := strings.TrimSpace(os.Getenv(roDSNEnv))
	if dsn == "" {
		t.Fatalf("%s=true but %s is empty; set the READ-ONLY DEV DSN", roTestEnv, roDSNEnv)
	}
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		t.Fatalf("open oracle: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping oracle: %v", err)
	}
	return &recordingQuerier{inner: oracle.NewReadOnlyDB(db)}
}

func roPeriod() string {
	if p := strings.TrimSpace(os.Getenv(roPeriodEnv)); p != "" {
		return p
	}
	return roDefaultPeriod
}

func assertAllGuarded(t *testing.T, q *recordingQuerier) {
	t.Helper()
	if len(q.stmts) == 0 {
		t.Fatal("no statements recorded")
	}
	for _, s := range q.stmts {
		if err := oracle.CheckReadOnly(s); err != nil {
			t.Errorf("statement not read-only: %v", err)
		}
		if strings.Contains(strings.ToUpper(s), "FOR UPDATE") {
			t.Errorf("statement contains FOR UPDATE: %s", s)
		}
	}
}

func TestRODev_AdjDemandBaseTables(t *testing.T) {
	q := openRODev(t)
	period := roPeriod()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	r, err := oracle.NewErpDemandReader(q, string(erpintegration.DemandSourceBaseTables), "development")
	if err != nil {
		t.Fatalf("new demand reader: %v", err)
	}
	rows, err := r.LoadAdjDemand(ctx, period)
	if err != nil {
		t.Fatalf("LoadAdjDemand(%s): %v", period, err)
	}
	if len(rows) == 0 {
		t.Fatalf("LoadAdjDemand(%s): 0 rows", period)
	}

	byTxn := map[string]int{}
	var heads, items, approved, posted int64
	for _, row := range rows {
		byTxn[row.TxnCode]++
		heads += row.HeadCount
		items += row.ItemCount
		approved += row.ApprovedItems
		posted += row.PostedItems
	}
	t.Logf("RO counts demand period=%s source=base_tables rows=%d byTxn=%v sumHeadCount=%d sumItems=%d approvedItems=%d postedItems=%d",
		period, len(rows), byTxn, heads, items, approved, posted)
	assertAllGuarded(t, q)
}

func TestRODev_AdjHeadProbe(t *testing.T) {
	q := openRODev(t)
	period := roPeriod()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := oracle.NewErpAdjReader(q).ProbeHeads(ctx, period)
	if err != nil {
		t.Fatalf("ProbeHeads(%s): %v", period, err)
	}
	if c.Heads <= 0 {
		t.Fatalf("ProbeHeads(%s): heads=%d, want > 0", period, c.Heads)
	}
	t.Logf("RO counts heads period=%s heads=%d posted=%d approved=%d nullStatus=%d eligible=%d",
		period, c.Heads, c.Posted, c.Approved, c.NullStatus, c.Eligible)
	assertAllGuarded(t, q)
}

func TestRODev_Master(t *testing.T) {
	q := openRODev(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	m := oracle.NewErpMasterReader(q)
	items, err := m.ListItems(ctx)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	grades, err := m.ListGrades(ctx)
	if err != nil {
		t.Fatalf("ListGrades: %v", err)
	}
	if len(items) == 0 || len(grades) == 0 {
		t.Fatalf("master empty: items=%d grades=%d", len(items), len(grades))
	}
	activeItems, activeGrades := 0, 0
	for _, it := range items {
		if it.Active {
			activeItems++
		}
	}
	for _, g := range grades {
		if g.Active {
			activeGrades++
		}
	}
	t.Logf("RO counts master items=%d activeItems=%d grades=%d activeGrades=%d",
		len(items), activeItems, len(grades), activeGrades)
	assertAllGuarded(t, q)
}
