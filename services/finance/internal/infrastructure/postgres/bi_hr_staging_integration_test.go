package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Gated by INTEGRATION_TEST=true. Applies migrations 000585 and 000592 (idempotent) and verifies the
// replace-in-window semantics (HD-04): rows inside the window are replaced, rows outside kept.
func TestBiHRStagingRepository_ReplaceOTDaily(t *testing.T) {
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

	for _, m := range []string{"000585_create_bi_hr_staging_ot", "000592_bi_hr_company"} {
		ddl, err := os.ReadFile("../../../migrations/postgres/" + m + ".up.sql")
		require.NoError(t, err)
		_, err = raw.ExecContext(ctx, string(ddl))
		require.NoError(t, err)
	}
	var srcID uuid.UUID
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE'`).Scan(&srcID))

	// Fixture dates far in the past so real data is never touched.
	const marker = "TST-H010"
	cleanup := func() {
		for _, q := range []string{
			`DELETE FROM bi_hr_stg_ot_daily WHERE nik = $1`,
			`DELETE FROM bi_hr_employee WHERE nik = $1`,
			`DELETE FROM bi_fact_metric WHERE type LIKE 'HR_OT_%' AND group_3 = $1`,
			`DELETE FROM bi_fact_metric WHERE type IN ('HR_OT_PARETO', 'HR_OT_EMP_COUNT', 'HR_OT_OVER_WEEKS', 'HR_OT_OVER_EMP', 'HR_OT_OVER_NIK') AND periode_date < '2000-01-01' AND $1 <> ''`,
		} {
			if _, cErr := raw.ExecContext(ctx, q, marker); cErr != nil {
				t.Logf("cleanup: %v", cErr)
			}
		}
	}
	cleanup()
	defer cleanup()

	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	row := func(date, typ, calc string) oracleinfra.OTDailyRow {
		return oracleinfra.OTDailyRow{
			WorkDate: d(date), WeekStart: d("1999-01-04"), AttPeriod: "199901", WeekAttPeriod: "199901",
			EmpSysID: "S1", NIK: marker, EmpName: "Test", DeptID: "D1", DeptName: "Dept",
			OTType: typ, Tier1: decimal.RequireFromString("1.5"), ActualHours: decimal.RequireFromString("1.5"),
			CalcHours: decimal.RequireFromString(calc), LimitHours: decimal.RequireFromString(calc),
		}
	}
	zero := func(date string) oracleinfra.OTDailyRow {
		r := row(date, "Normal", "0")
		r.Tier1, r.ActualHours = decimal.Zero, decimal.Zero
		return r
	}
	repo := postgres.NewBiHRStagingRepository(&postgres.DB{DB: raw})

	// Staging must cover period 199901 (1998-12-26..1999-01-25) from its first day, else MONTHLY is
	// skipped as partial; the zero-hour 12-26 row anchors it.
	// The latest work_date carries the current division/dept (HD-18), even when read first.
	latest := row("1999-01-05", "Change Off", "7.5")
	latest.Division, latest.DeptName, latest.CompID, latest.CompName = "DIV-NEW", "Dept", "MGT", "PT. Mutu Gading Tekstil"
	older := row("1999-01-04", "Normal", "2.25")
	older.Division, older.DeptName = "DIV-OLD", "Dept Lama"
	n, err := repo.ReplaceOTDaily(ctx, d("1998-12-26"), d("1999-01-10"),
		[]oracleinfra.OTDailyRow{latest, zero("1998-12-26"), older}, srcID)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	var div, dept, compID, comp string
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT COALESCE(division, ''), COALESCE(dept_name, ''), COALESCE(company_id, ''), COALESCE(company, '')
FROM bi_hr_employee WHERE nik = $1`, marker).Scan(&div, &dept, &compID, &comp))
	require.Equal(t, "DIV-NEW", div)
	require.Equal(t, "Dept", dept)
	require.Equal(t, "MGT", compID)
	require.Equal(t, "PT. Mutu Gading Tekstil", comp)
	var stgComp, nullComp int // HD-19: company kept per staging row; empty -> NULL
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE company_id = 'MGT' AND company = 'PT. Mutu Gading Tekstil'),
       COUNT(*) FILTER (WHERE company_id IS NULL AND company IS NULL)
FROM bi_hr_stg_ot_daily WHERE nik = $1`, marker).Scan(&stgComp, &nullComp))
	require.Equal(t, 1, stgComp)
	require.Equal(t, 2, nullComp)

	// Second run over a sub-window: the 01-05 row is replaced, 01-04 (outside) kept.
	n, err = repo.ReplaceOTDaily(ctx, d("1999-01-05"), d("1999-01-10"),
		[]oracleinfra.OTDailyRow{row("1999-01-06", "Normal", "3.125")}, srcID)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var cnt int
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM bi_hr_stg_ot_daily WHERE nik = $1`, marker).Scan(&cnt))
	require.Equal(t, 3, cnt)
	var calc string
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT ot_calc_hours::text FROM bi_hr_stg_ot_daily WHERE nik = $1 AND work_date = '1999-01-06'`, marker).Scan(&calc))
	require.True(t, decimal.RequireFromString(calc).Equal(decimal.RequireFromString("3.125")))
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM bi_hr_employee WHERE nik = $1`, marker).Scan(&cnt))
	require.Equal(t, 1, cnt)

	// H011: facts follow staging in the same transaction. Rows: 01-04 Normal 1.5 (kept) and
	// 01-06 Normal 1.5 (calc 3.125 → DQ 1.625); the 01-05 Change Off row is gone (HD-04).
	// r2 selects the R2 types (H012) instead of the R1 ones.
	factsOf := func(r2 bool) map[string]string {
		rs, qErr := raw.QueryContext(ctx, `SELECT type||' '||periode_grain||' '||periode_label||' '||COALESCE(group_2,''), value::text
FROM bi_fact_metric WHERE type LIKE 'HR_OT_%' AND group_3 = $1
  AND (type IN ('HR_OT_HOURS', 'HR_OT_DQ')) <> $2`, marker, r2)
		require.NoError(t, qErr)
		defer func() { _ = rs.Close() }()
		m := map[string]string{}
		for rs.Next() {
			var k, v string
			require.NoError(t, rs.Scan(&k, &v))
			m[k] = decimal.RequireFromString(v).String()
		}
		require.NoError(t, rs.Err())
		return m
	}
	facts := func() map[string]string { return factsOf(false) }
	want := map[string]string{
		"HR_OT_HOURS DAILY 1999-01-04 Normal": "1.5",
		"HR_OT_HOURS DAILY 1999-01-06 Normal": "1.5",
		"HR_OT_HOURS MONTHLY 199901 Normal":   "3",
		"HR_OT_DQ DAILY 1999-01-04 Normal":    "0.75",
		"HR_OT_DQ DAILY 1999-01-06 Normal":    "1.625",
		"HR_OT_DQ MONTHLY 199901 Normal":      "2.375",
	}
	require.Equal(t, want, facts())
	// H032: one employee with OT in period 199901 → HR_OT_EMP_COUNT 1 under the current dept, uom PERSON.
	empCount := func() (string, string) {
		var v, uom string
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT COALESCE(SUM(value), 0)::text, COALESCE(MAX(group_1||'/'||uom), '')
FROM bi_fact_metric WHERE type = 'HR_OT_EMP_COUNT' AND periode_grain = 'MONTHLY' AND periode_date = '1999-01-25'`).Scan(&v, &uom))
		return decimal.RequireFromString(v).String(), uom
	}
	n1, u1 := empCount()
	require.Equal(t, "1", n1)
	require.Equal(t, "Dept/PERSON", u1)

	// Idempotent: the same run twice yields identical facts (ACCEPTANCE 1.1).
	_, err = repo.ReplaceOTDaily(ctx, d("1999-01-05"), d("1999-01-10"),
		[]oracleinfra.OTDailyRow{row("1999-01-06", "Normal", "3.125")}, srcID)
	require.NoError(t, err)
	require.Equal(t, want, facts())
	n1, _ = empCount()
	require.Equal(t, "1", n1)

	// H012: R2 week 1999-W01 (Mon 01-04) = 2.25 + 3.125 counted hours; week 1998-W51 (Mon 12-21)
	// starts before staging does and is skipped; the removed 01-05 day fact is gone.
	require.Equal(t, map[string]string{
		"HR_OT_WEEK WEEKLY 1999-W01 UNDER_LIMIT":   "5.375",
		"HR_OT_WEEK_BUCKET WEEKLY 1999-W01 0-7":    "1",
		"HR_OT_WEEK_DAILY DAILY 1999-01-04 Normal": "2.25",
		"HR_OT_WEEK_DAILY DAILY 1999-01-06 Normal": "3.125",
	}, factsOf(true))

	// Over-limit in two consecutive runs: the streak continues from the stored HR_OT_CHRONIC of the
	// week before the window; Holiday counts only above 7.5 h (8 → 0.5).
	_, err = repo.ReplaceOTDaily(ctx, d("1999-01-11"), d("1999-01-17"),
		[]oracleinfra.OTDailyRow{row("1999-01-12", "Normal", "15")}, srcID)
	require.NoError(t, err)
	_, err = repo.ReplaceOTDaily(ctx, d("1999-01-18"), d("1999-01-24"),
		[]oracleinfra.OTDailyRow{row("1999-01-19", "Off", "14"), row("1999-01-23", "Holiday", "8")}, srcID)
	require.NoError(t, err)
	r2 := factsOf(true)
	for k, v := range map[string]string{
		"HR_OT_WEEK WEEKLY 1999-W02 OVER_LIMIT":        "15",
		"HR_OT_WEEK_STATUS WEEKLY 1999-W02 OVER_LIMIT": "1",
		"HR_OT_WEEK_EXCESS WEEKLY 1999-W02 ":           "1",
		"HR_OT_CHRONIC WEEKLY 1999-W02 ":               "1",
		"HR_OT_WEEK WEEKLY 1999-W03 OVER_LIMIT":        "14.5",
		"HR_OT_WEEK_BUCKET WEEKLY 1999-W03 14-18":      "1",
		"HR_OT_CHRONIC WEEKLY 1999-W03 ":               "2",
		"HR_OT_WEEK_DAILY DAILY 1999-01-23 Holiday":    "0.5",
	} {
		require.Equal(t, v, r2[k], k)
	}

	// H038 / HD-20: W02 and W03 both end on a Sunday inside payroll period 199901 (26 Dec–25 Jan):
	// 2 over-limit employee-weeks, 1 distinct employee; W01 is under the limit and not counted.
	overMonth := func(typ string) string {
		var v string
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT COALESCE(SUM(value), 0)::text FROM bi_fact_metric
WHERE type = $1 AND periode_grain = 'MONTHLY' AND periode_date = '1999-01-25' AND group_1 = 'Dept'`, typ).Scan(&v))
		return decimal.RequireFromString(v).String()
	}
	require.Equal(t, "2", overMonth("HR_OT_OVER_WEEKS"))
	require.Equal(t, "1", overMonth("HR_OT_OVER_EMP"))
	// H042 / HD-22: per-NIK weeks sum to the dept orang-minggu (selisih 0).
	require.Equal(t, "2", overMonth("HR_OT_OVER_NIK"))

	// HD-15: an employee's facts outside the window follow the current dept.
	_, err = raw.ExecContext(ctx, `UPDATE bi_hr_employee SET dept_name = 'Dept Baru' WHERE nik = $1`, marker)
	require.NoError(t, err)
	_, err = repo.ReplaceOTDaily(ctx, d("1999-02-01"), d("1999-02-02"), nil, srcID)
	require.NoError(t, err)
	var depts []string
	rs, err := raw.QueryContext(ctx, `SELECT DISTINCT group_1 FROM bi_fact_metric WHERE type LIKE 'HR_OT_%' AND group_3 = $1`, marker)
	require.NoError(t, err)
	for rs.Next() {
		var g string
		require.NoError(t, rs.Scan(&g))
		depts = append(depts, g)
	}
	require.NoError(t, rs.Close())
	require.Equal(t, []string{"Dept Baru"}, depts)
}
