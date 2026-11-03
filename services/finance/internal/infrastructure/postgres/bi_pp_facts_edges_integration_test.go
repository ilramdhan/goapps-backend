package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// ppEdgeEnv is the shared setup of the window-edge / failure-path PP fact tests (P007).
type ppEdgeEnv struct {
	ctx   context.Context
	raw   *sql.DB
	repo  *postgres.BiPPStagingRepository
	srcID uuid.UUID
}

func newPPEdgeEnv(t *testing.T) *ppEdgeEnv {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 30*time.Second))
	t.Cleanup(func() { _ = raw.Close() })

	srcID := hrisOracleSourceID(ctx, t, raw)
	cleanupPPFixtures(ctx, t, raw)
	t.Cleanup(func() { cleanupPPFixtures(ctx, t, raw) })
	return &ppEdgeEnv{ctx: ctx, raw: raw, repo: postgres.NewBiPPStagingRepository(&postgres.DB{DB: raw}), srcID: srcID}
}

func ppDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	require.NoError(t, err)
	return d
}

// replace loads fixtures into the [d1, d2] window through the normal job path.
func (e *ppEdgeEnv) replace(t *testing.T, d1, d2 string, fx ...ppFixture) {
	t.Helper()
	rows := make([]oracleinfra.PackProdDailyRow, 0, len(fx))
	for _, f := range fx {
		rows = append(rows, f.row())
	}
	_, err := e.repo.ReplacePackProdDaily(e.ctx, ppDate(t, d1), ppDate(t, d2), rows, e.srcID)
	require.NoError(t, err)
}

// summaryTotal reads the TWISTING PP_PACK_SUMMARY PACK_TOTAL fact of one grain/period ("" when absent).
func (e *ppEdgeEnv) summaryTotal(t *testing.T, grain, date string) string {
	t.Helper()
	var v string
	err := e.raw.QueryRowContext(e.ctx, `SELECT value::text FROM bi_fact_metric
		WHERE type = 'PP_PACK_SUMMARY' AND group_1 = 'TWISTING' AND metric_name = 'PACK_TOTAL'
		  AND periode_grain = $1 AND periode_date = $2::date`, grain, date).Scan(&v)
	if err == sql.ErrNoRows {
		return ""
	}
	require.NoError(t, err)
	return v
}

func ppEdgeRow(date, fq string) ppFixture {
	return ppFixture{date: date, dept: "TWT", txType: "PACKING TW", txTypeMC: "PACKING TW MC", groupType: "Regular",
		mergeNo: "M1", mcNo: "MC1", poly: "P1", eligible: "Y", prdOrder: 3, groupOrder: 2,
		ax: "0", fq: fq, dg: "0", jlt: "0"}
}

// Gated by INTEGRATION_TEST=true. A window ending mid-week (Tuesday 1999-03-09) still recomputes that
// week completely from staging (including Wednesday 03-10, after d2), and leaves weeks whose Monday
// is after d2 untouched: WEEKLY is bounded by week_start <= d2, not by the week's last day.
func TestBiPPStagingRepository_ReplacePackProdDaily_WeeklyRightEdge(t *testing.T) {
	e := newPPEdgeEnv(t)

	e.replace(t, "1999-03-08", "1999-03-19", ppEdgeRow("1999-03-08", "1"), ppEdgeRow("1999-03-10", "1"), ppEdgeRow("1999-03-16", "1"))
	require.Equal(t, "2.0000", e.summaryTotal(t, "WEEKLY", "1999-03-08"))
	require.Equal(t, "1.0000", e.summaryTotal(t, "WEEKLY", "1999-03-15"))

	// Sentinel on the week after d2: a correct upper bound must not touch (or recompute) it.
	_, err := e.raw.ExecContext(e.ctx, `UPDATE bi_fact_metric SET value = 999 WHERE type = 'PP_PACK_SUMMARY'
		AND metric_name = 'PACK_TOTAL' AND periode_grain = 'WEEKLY' AND periode_date = DATE '1999-03-15'`)
	require.NoError(t, err)

	e.replace(t, "1999-03-08", "1999-03-09", ppEdgeRow("1999-03-08", "5"))

	require.Equal(t, "6.0000", e.summaryTotal(t, "WEEKLY", "1999-03-08"), "week cut by d2 must include 03-10 from staging")
	require.Equal(t, "999.0000", e.summaryTotal(t, "WEEKLY", "1999-03-15"), "week starting after d2 must stay untouched")
	require.Equal(t, "1.0000", e.summaryTotal(t, "DAILY", "1999-03-10"), "daily after d2 must stay untouched")
}

// stageRow inserts one weekly-consistent window, then makes staging finer than the 4dp fact column
// behind the facts' back, so any recompute must fail the ETL§F reconciliation.
func (e *ppEdgeEnv) stageUnrepresentable(t *testing.T) map[string]ppFact {
	t.Helper()
	e.replace(t, "1999-03-08", "1999-03-12", ppEdgeRow("1999-03-08", "1"))
	before := readPPFacts(e.ctx, t, e.raw)
	require.NotEmpty(t, before)
	_, err := e.raw.ExecContext(e.ctx, `UPDATE bi_pp_stg_pack_prod_daily SET fq_qty = 0.00005, total_qty = 0.00005
		WHERE pack_date = DATE '1999-03-08'`)
	require.NoError(t, err)
	return before
}

// RebuildPackProdFacts must fail on a reconciliation mismatch, name the WEEKLY period in the error,
// roll back (facts unchanged) and leave staging alone.
func TestBiPPStagingRepository_RebuildPackProdFacts_ReconMismatch_RollsBack(t *testing.T) {
	e := newPPEdgeEnv(t)
	before := e.stageUnrepresentable(t)

	_, err := e.repo.RebuildPackProdFacts(e.ctx, ppDate(t, "1999-03-08"), ppDate(t, "1999-03-12"), e.srcID)
	require.ErrorContains(t, err, "reconcile")
	require.ErrorContains(t, err, "WEEKLY 1999-03-08")
	require.Equal(t, before, readPPFacts(e.ctx, t, e.raw))

	var n int
	require.NoError(t, e.raw.QueryRowContext(e.ctx,
		`SELECT count(*) FROM bi_pp_stg_pack_prod_daily WHERE pack_date = DATE '1999-03-08'`).Scan(&n))
	require.Equal(t, 1, n, "rebuild must not touch staging")
}

// The WEEKLY side of the ETL§F reconciliation (PP-08) catches a mismatch on its own: the job path
// reports the WEEKLY period too and rolls the whole load back.
func TestBiPPStagingRepository_ReplacePackProdDaily_WeeklyReconMismatch_RollsBack(t *testing.T) {
	e := newPPEdgeEnv(t)
	e.replace(t, "1999-03-08", "1999-03-12", ppEdgeRow("1999-03-08", "1"))
	before := readPPFacts(e.ctx, t, e.raw)

	fine := ppEdgeRow("1999-03-08", "0.00005")
	_, err := e.repo.ReplacePackProdDaily(e.ctx, ppDate(t, "1999-03-08"), ppDate(t, "1999-03-12"),
		[]oracleinfra.PackProdDailyRow{fine.row()}, e.srcID)
	require.ErrorContains(t, err, "WEEKLY 1999-03-08")
	require.Equal(t, before, readPPFacts(e.ctx, t, e.raw))
}
