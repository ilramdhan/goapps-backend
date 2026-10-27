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

// Gated by INTEGRATION_TEST=true. Applies migration 000593 (idempotent) and checks that the replace
// is windowed (HD-04) and HR_HEADCOUNT counts distinct NIK per payroll period (H016).
// Run with -run TestBiHRStagingRepository_ReplaceATTDaily only.
func TestBiHRStagingRepository_ReplaceATTDaily(t *testing.T) {
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

	ddl, err := os.ReadFile("../../../migrations/postgres/000593_bi_hr_stg_att_daily.up.sql")
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, string(ddl))
	require.NoError(t, err)

	var srcID uuid.UUID
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE'`).Scan(&srcID))

	// Fixture in 1999 so real data is never touched; headcount facts carry no NIK, so they are
	// cleaned by date.
	cleanup := func() {
		for _, q := range []string{
			`DELETE FROM bi_hr_stg_att_daily WHERE nik LIKE 'TST-H016-%'`,
			`DELETE FROM bi_fact_metric WHERE type = 'HR_HEADCOUNT' AND periode_date < '2000-01-01'`,
		} {
			if _, cErr := raw.ExecContext(ctx, q); cErr != nil {
				t.Logf("cleanup: %v", cErr)
			}
		}
	}
	cleanup()
	defer cleanup()

	repo := postgres.NewBiHRStagingRepository(&postgres.DB{DB: raw})
	day := func(s string) time.Time {
		v, pErr := time.Parse("2006-01-02", s)
		require.NoError(t, pErr)
		return v
	}
	row := func(date, nik, status string) oracleinfra.ATTDailyRow {
		return oracleinfra.ATTDailyRow{
			WorkDate: day(date), WeekStart: day(date), AttPeriod: "199903", NIK: nik, EmpSysID: nik,
			DeptName: "Weaving", EmpStatus: status, ActualHour: decimal.NewFromFloat(8),
		}
	}
	// Period 26 Feb–25 Mar 1999: A on two days, B on one day; 25 Feb belongs to the previous period.
	rows := []oracleinfra.ATTDailyRow{
		row("1999-02-25", "TST-H016-A", "Tetap"),
		row("1999-03-01", "TST-H016-A", "Tetap"),
		row("1999-03-02", "TST-H016-A", "Tetap"),
		row("1999-03-02", "TST-H016-B", "Kontrak"),
	}
	n, err := repo.ReplaceATTDaily(ctx, day("1999-02-25"), day("1999-03-05"), rows, srcID)
	require.NoError(t, err)
	require.Equal(t, 4, n)

	count := func(grain, date, status string) int {
		var v sql.NullInt64
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT SUM(value)::bigint FROM bi_fact_metric
WHERE type = 'HR_HEADCOUNT' AND periode_grain = $1 AND periode_date = $2 AND group_1 = 'Weaving' AND group_2 = $3`,
			grain, date, status).Scan(&v))
		return int(v.Int64)
	}
	require.Equal(t, 1, count("MONTHLY", "1999-03-25", "Tetap"), "A counts once for the period")
	require.Equal(t, 1, count("MONTHLY", "1999-03-25", "Kontrak"))
	require.Equal(t, 0, count("MONTHLY", "1999-02-25", "Tetap"), "period 26 Jan-25 Feb is only partly staged: no MONTHLY fact")
	require.Equal(t, 1, count("DAILY", "1999-02-25", "Tetap"))
	require.Equal(t, 2, count("DAILY", "1999-03-02", "Tetap")+count("DAILY", "1999-03-02", "Kontrak"))

	// Re-run with B removed at the source: replaced inside the window, facts follow.
	n, err = repo.ReplaceATTDaily(ctx, day("1999-02-25"), day("1999-03-05"), rows[:3], srcID)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, 0, count("MONTHLY", "1999-03-25", "Kontrak"))
	require.Equal(t, 1, count("MONTHLY", "1999-03-25", "Tetap"))
}
