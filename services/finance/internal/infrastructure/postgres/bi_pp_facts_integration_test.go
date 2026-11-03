package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// hrisOracleSourceID returns the HRIS_ORACLE data source (migration 000582) the PP facts are stamped with.
func hrisOracleSourceID(ctx context.Context, t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE'`).Scan(&id),
		"HRIS_ORACLE must exist: run the finance migrations first")
	return id
}

// cleanupPPFixtures removes the pre-2000 fixture rows (view history starts 2003, so real data is
// never touched) from staging and facts.
func cleanupPPFixtures(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`DELETE FROM bi_pp_stg_pack_prod_daily WHERE pack_date < '2000-01-01'`,
		`DELETE FROM bi_fact_metric WHERE type LIKE 'PP\_PACK\_%' AND periode_date < '2000-01-01'`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
}

// ppFixture is the subset of view columns the fact aggregation reads.
type ppFixture struct {
	date, dept, txType, txTypeMC, groupType, mergeNo, mcNo, poly, eligible string
	prdOrder, groupOrder                                                   int64
	ax, fq, dg, jlt                                                        string
}

func (f ppFixture) row() oracleinfra.PackProdDailyRow {
	d, err := time.Parse(time.DateOnly, f.date)
	if err != nil {
		panic(err)
	}
	monday := d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
	num := func(s string) decimal.Decimal {
		if s == "" {
			return decimal.Zero
		}
		return decimal.RequireFromString(s)
	}
	deptName := map[string]string{"TWT": "TWISTING", "TXT": "TEXTURISING"}[f.dept]
	ax, fq, dg, jlt := num(f.ax), num(f.fq), num(f.dg), num(f.jlt)
	return oracleinfra.PackProdDailyRow{
		PackDate: d, WeekStart: monday, MonthStart: time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC),
		Period: d.Format("200601"), WeekPeriod: d.Format("200601"),
		DeptID: f.dept, DeptName: deptName, ProdGroup: f.dept,
		PrdTypeOrder:      sql.NullInt64{Int64: f.prdOrder, Valid: true},
		TransactionType:   str(f.txType),
		TransactionTypeMC: str(f.txTypeMC),
		GroupType:         str(f.groupType),
		GroupTypeOrder:    sql.NullInt64{Int64: f.groupOrder, Valid: true},
		PolyType:          str(f.poly), McNo: str(f.mcNo), MergeNo: str(f.mergeNo),
		GradeEligible: f.eligible,
		AxQty:         ax, FqQty: fq, DgQty: dg, JltQty: jlt,
		TotalQty: fq.Add(dg).Add(jlt), // FQ includes AX
	}
}

// ppFact is one fact row as read back for comparison (loaded_at excluded).
type ppFact struct {
	Type, G1, G2, G3, Grain, Date, Label, Metric, Value, Dim string
	O1, O2, O3                                               sql.NullInt64
}

func readPPFacts(ctx context.Context, t *testing.T, db *sql.DB) map[string]ppFact {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT type, group_1, COALESCE(group_2, '~'), COALESCE(group_3, '~'),
		periode_grain, periode_date::text, periode_label, metric_name, value::text, dimension_key,
		group_1_order, group_2_order, group_3_order
		FROM bi_fact_metric WHERE type LIKE 'PP\_PACK\_%' AND periode_date < '2000-01-01'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]ppFact{}
	for rows.Next() {
		var f ppFact
		require.NoError(t, rows.Scan(&f.Type, &f.G1, &f.G2, &f.G3, &f.Grain, &f.Date, &f.Label, &f.Metric,
			&f.Value, &f.Dim, &f.O1, &f.O2, &f.O3))
		k := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s", f.Type, f.G1, f.G2, f.G3, f.Dim, f.Grain, f.Date, f.Metric)
		_, dup := out[k]
		require.False(t, dup, "duplicate fact %s", k)
		out[k] = f
	}
	require.NoError(t, rows.Err())
	return out
}

// Gated by INTEGRATION_TEST=true. Needs a database migrated through 000602 (bi_fact_metric,
// HRIS_ORACLE, PACK_* registry). Verifies ETL§D (DAILY/WEEKLY/MONTHLY, PP-06/P007) inside the staging
// transaction: per-type filters, group labels/orders, 5 long-format metrics, idempotency, rows
// dropped at the source disappearing, month-cutting windows recomputing the whole month from
// staging, and the ETL§F reconciliation (a mismatch would fail ReplacePackProdDaily).
func TestBiPPStagingRepository_ReplacePackProdDaily_Facts(t *testing.T) {
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
	defer func() { _ = raw.Close() }()

	srcID := hrisOracleSourceID(ctx, t, raw)
	cleanupPPFixtures(ctx, t, raw)
	defer cleanupPPFixtures(ctx, t, raw)

	repo := postgres.NewBiPPStagingRepository(&postgres.DB{DB: raw})
	d := func(s string) time.Time { v, _ := time.Parse(time.DateOnly, s); return v }
	load := func(d1, d2 string, fx ...ppFixture) {
		t.Helper()
		rows := make([]oracleinfra.PackProdDailyRow, 0, len(fx))
		for _, f := range fx {
			rows = append(rows, f.row())
		}
		_, lErr := repo.ReplacePackProdDaily(ctx, d(d1), d(d2), rows, srcID)
		require.NoError(t, lErr)
	}

	twt := ppFixture{dept: "TWT", txType: "PACKING TW", txTypeMC: "PACKING TW MC", groupType: "Regular",
		mergeNo: "M1", mcNo: "MC1", poly: "P1", eligible: "Y", prdOrder: 3, groupOrder: 2,
		ax: "10.25", fq: "12.50", dg: "1.25", jlt: "0.01"}
	at := func(f ppFixture, date string) ppFixture { f.date = date; return f }

	// Early January, loaded by an earlier run (outside the later daily window).
	early := at(twt, "1999-01-03")
	// early2 (Saturday 1999-01-09) shares week 1999-W01 with the first window day (Sunday 01-10).
	early2 := at(twt, "1999-01-09")
	load("1999-01-01", "1999-01-09", early, early2)

	// Window 1999-01-10..1999-02-05 cuts January: its MONTHLY must include 1999-01-03 from staging.
	txt := ppFixture{dept: "TXT", txType: "PACKING TX", txTypeMC: "PACKING TX MC", groupType: "Rewinding",
		mergeNo: "M2", mcNo: "MC9", poly: "P2", eligible: "N", prdOrder: 1, groupOrder: 5,
		ax: "100", fq: "200", dg: "0", jlt: "3.5"}
	noMerge := at(twt, "1999-01-12")
	noMerge.mergeNo, noMerge.ax, noMerge.fq, noMerge.dg, noMerge.jlt = "", "0", "1", "0", "0"
	window := []ppFixture{at(twt, "1999-01-12"), at(txt, "1999-01-12"), noMerge, at(twt, "1999-02-02")}
	load("1999-01-10", "1999-02-05", window...)
	first := readPPFacts(ctx, t, raw)

	key := func(typ, g1, g2, g3, dim, grain, date, metric string) string {
		return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s", typ, g1, g2, g3, dim, grain, date, metric)
	}
	val := func(k string) string {
		t.Helper()
		f, ok := first[k]
		require.True(t, ok, "missing fact %s", k)
		return f.Value
	}

	// SUMMARY January TWT = 1999-01-03, 01-09 + two 1999-01-12 TWT rows (whole month from staging).
	require.Equal(t, "42.2800", val(key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-01-01", "PACK_TOTAL")))
	require.Equal(t, "30.7500", val(key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-01-01", "PACK_AX")))
	require.Equal(t, "199901", first[key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-01-01", "PACK_AX")].Label)
	// The early daily fact is outside the window and untouched; DAILY label is YYYY-MM-DD.
	require.Equal(t, "13.7600", val(key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "DAILY", "1999-01-03", "PACK_TOTAL")))
	require.Equal(t, "1999-01-03", first[key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "DAILY", "1999-01-03", "PACK_TOTAL")].Label)
	// 5 metrics in long format, orders TWT=1/TXT=2, prd/group orders copied.
	txtSum := first[key("PP_PACK_SUMMARY", "TEXTURISING", "PACKING TX", "Rewinding", "", "DAILY", "1999-01-12", "PACK_JLT")]
	require.Equal(t, "3.5000", txtSum.Value)
	require.Equal(t, int64(2), txtSum.O1.Int64)
	require.Equal(t, int64(1), txtSum.O2.Int64)
	require.Equal(t, int64(5), txtSum.O3.Int64)
	require.Equal(t, int64(1), first[key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "DAILY", "1999-01-12", "PACK_DG")].O1.Int64)
	// PRODUCT: Rewinding excluded, empty merge_no keyed ''.
	require.NotContains(t, first, key("PP_PACK_PRODUCT", "TEXTURISING", "PACKING TX", "Rewinding", "M2", "DAILY", "1999-01-12", "PACK_TOTAL"))
	require.Equal(t, "13.7600", val(key("PP_PACK_PRODUCT", "TWISTING", "PACKING TW", "Regular", "M1", "DAILY", "1999-01-12", "PACK_TOTAL")))
	require.Equal(t, "1.0000", val(key("PP_PACK_PRODUCT", "TWISTING", "PACKING TW", "Regular", "", "DAILY", "1999-01-12", "PACK_TOTAL")))
	// MACHINE: grade_eligible='Y' only, group_2 = transaction_type_mc.
	require.NotContains(t, first, key("PP_PACK_MACHINE", "TEXTURISING", "PACKING TX MC", "Rewinding", "MC9", "DAILY", "1999-01-12", "PACK_TOTAL"))
	require.Equal(t, "42.2800", val(key("PP_PACK_MACHINE", "TWISTING", "PACKING TW MC", "Regular", "MC1", "MONTHLY", "1999-01-01", "PACK_TOTAL")))
	// POLY: all rows, group_2 = transaction_type_mc.
	require.Equal(t, "203.5000", val(key("PP_PACK_POLY", "TEXTURISING", "PACKING TX MC", "Rewinding", "P2", "MONTHLY", "1999-01-01", "PACK_TOTAL")))
	// WEEKLY (P007): week_start Monday, label IYYY-"W"IW. The window (starting Sunday 01-10) cuts
	// week 01-04: it still sums early2 from staging; week 12-28 (ISO year 1998, W53, from the Sunday
	// 01-03 load) lies before the window and stays untouched.
	sumTW := func(week string) string {
		return key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "WEEKLY", week, "PACK_TOTAL")
	}
	require.Equal(t, "13.7600", val(sumTW("1998-12-28")))
	require.Equal(t, "1998-W53", first[sumTW("1998-12-28")].Label)
	require.Equal(t, "13.7600", val(sumTW("1999-01-04")))
	require.Equal(t, "1999-W01", first[sumTW("1999-01-04")].Label)
	require.Equal(t, "14.7600", val(sumTW("1999-01-11")))
	require.Equal(t, "1999-W02", first[sumTW("1999-01-11")].Label)
	require.Equal(t, "13.7600", val(sumTW("1999-02-01")))
	require.Equal(t, "14.7600", val(key("PP_PACK_MACHINE", "TWISTING", "PACKING TW MC", "Regular", "MC1", "WEEKLY", "1999-01-11", "PACK_TOTAL")))
	perMetric := map[string]int{}
	for _, f := range first {
		perMetric[f.Metric]++
	}
	require.Len(t, perMetric, 5)
	for m, c := range perMetric {
		require.Equal(t, perMetric["PACK_TOTAL"], c, "metric %s count", m)
	}

	// Second identical run: identical facts.
	load("1999-01-10", "1999-02-05", window...)
	require.Equal(t, first, readPPFacts(ctx, t, raw))

	// P007 one-shot: WEEKLY history missing for staged days is rebuilt from staging alone.
	_, err = raw.ExecContext(ctx, `DELETE FROM bi_fact_metric WHERE periode_grain = 'WEEKLY' AND type LIKE 'PP_PACK_%' AND periode_date < DATE '2000-01-01'`)
	require.NoError(t, err)
	_, err = repo.RebuildPackProdFacts(ctx, d("1999-01-01"), d("1999-02-05"), srcID)
	require.NoError(t, err)
	require.Equal(t, first, readPPFacts(ctx, t, raw))

	// Source drops the TXT row: its facts vanish, January MONTHLY is recomputed, early daily stays.
	load("1999-01-10", "1999-02-05", window[0], window[2], window[3])
	got := readPPFacts(ctx, t, raw)
	for k, f := range got {
		require.NotEqual(t, "TEXTURISING", f.G1, "fact %s should be gone", k)
	}
	require.Equal(t, first[key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-01-01", "PACK_TOTAL")],
		got[key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-01-01", "PACK_TOTAL")])
	require.Contains(t, got, key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "DAILY", "1999-01-03", "PACK_TOTAL"))

	// An empty source empties the window, but January MONTHLY still counts the early staging day.
	load("1999-01-10", "1999-02-05")
	got = readPPFacts(ctx, t, raw)
	require.Equal(t, "27.5200", got[key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-01-01", "PACK_TOTAL")].Value)
	require.Equal(t, "13.7600", got[sumTW("1999-01-04")].Value)
	require.NotContains(t, got, sumTW("1999-01-11"))
	require.NotContains(t, got, sumTW("1999-02-01"))
	require.NotContains(t, got, key("PP_PACK_SUMMARY", "TWISTING", "PACKING TW", "Regular", "", "MONTHLY", "1999-02-01", "PACK_TOTAL"))

	// The ETL§F reconciliation does catch a mismatch (PP-08): a staging value finer than the 4dp
	// fact column cannot be represented, so the whole load rolls back and the previous state stays.
	fine := at(twt, "1999-01-12")
	fine.ax, fine.fq, fine.dg, fine.jlt = "0", "0.00005", "0", "0"
	_, err = repo.ReplacePackProdDaily(ctx, d("1999-01-10"), d("1999-02-05"), []oracleinfra.PackProdDailyRow{fine.row()}, srcID)
	require.ErrorContains(t, err, "reconcile")
	require.Equal(t, got, readPPFacts(ctx, t, raw))
}
