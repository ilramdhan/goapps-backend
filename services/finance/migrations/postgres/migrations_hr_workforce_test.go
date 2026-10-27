// Migration round-trip tests for the HR Workforce chain (000582–000597): grain WEEKLY (583), the
// WORKFORCE dashboards (588) and the final shape of HR_OT_TREND after the squashed 596/597.
// Local PostgreSQL only, via internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const (
	preHRWorkforce   uint64 = 581 // last migration before the HR chain
	weeklyGrainVer   uint64 = 583
	workforceDashVer uint64 = 588
	preHROTTrendVer  uint64 = 595 // before the squashed 596/597
	hrOTOverMonthVer uint64 = 597
)

// insertWeeklyFact inserts one WEEKLY fact row (source HRIS_ORACLE from 000582).
const insertWeeklyFact = `INSERT INTO bi_fact_metric (type, group_1, periode_grain, periode_date, periode_label, value, display_value, source_id)
	SELECT 'HR_T583', 'PRD', 'WEEKLY', DATE '2026-10-05', '2026-W41', 1, 1, source_id
	FROM bi_data_source WHERE source_code = 'HRIS_ORACLE'`

func TestHRWorkforceMigrations(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "hr_workforce_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), hrOTOverMonthVer)
	require.NoError(t, m.UpTo(ctx, preHRWorkforce))

	t.Run("583 weekly grain up/down/up", func(t *testing.T) {
		require.NoError(t, m.UpTo(ctx, weeklyGrainVer))
		mustExec(ctx, t, db, insertWeeklyFact)
		assert.True(t, constraintValidated(ctx, t, db, "bi_fact_metric_periode_grain_check"), "NOT VALID must be validated")

		require.NoError(t, m.DownTo(ctx, weeklyGrainVer-1))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_fact_metric WHERE periode_grain = 'WEEKLY'`))
		assertExecFails(ctx, t, db, insertWeeklyFact, "bi_fact_metric_periode_grain_check")

		require.NoError(t, m.UpTo(ctx, weeklyGrainVer))
		mustExec(ctx, t, db, insertWeeklyFact)
		mustExec(ctx, t, db, `DELETE FROM bi_fact_metric WHERE type = 'HR_T583'`)
	})

	t.Run("588 workforce dashboards up/down/up", func(t *testing.T) {
		require.NoError(t, m.UpTo(ctx, workforceDashVer))
		assertWorkforce588(ctx, t, db)

		require.NoError(t, m.DownTo(ctx, workforceDashVer-1))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard WHERE dashboard_code LIKE 'HR_%'`))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard_group WHERE group_code = 'WORKFORCE'`))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_job WHERE job_name = 'ETL_HR_OT_MIDDAY'`))
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bi_job WHERE job_name = 'ETL_HR_OT_DAILY' AND schedule_cron = '0 5 * * *'`))

		require.NoError(t, m.UpTo(ctx, workforceDashVer))
		assertWorkforce588(ctx, t, db)
	})

	t.Run("final HR_OT_TREND shape", func(t *testing.T) {
		require.NoError(t, m.Up(ctx))
		assertTrendFinal(ctx, t, db)
	})

	t.Run("596/597 idempotent re-run", func(t *testing.T) {
		before := hrDashboardSnapshot(ctx, t, db)
		for _, f := range []string{"000596_bi_hr_ot_trend.up.sql", "000597_bi_hr_ot_over_month.up.sql"} {
			require.NoError(t, pgcontainer.ExecFile(ctx, db, f))
		}
		assert.Equal(t, before, hrDashboardSnapshot(ctx, t, db))
	})

	t.Run("596/597 down/up", func(t *testing.T) {
		require.NoError(t, m.DownTo(ctx, preHROTTrendVer))
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard WHERE dashboard_code = 'HR_OT_TREND'
			AND periode_grain = 'DAILY' AND chart_type = 'line' AND layout_config IS NULL AND NOT drill_enabled`))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard
			WHERE dashboard_code IN ('HR_OT_PARETO', 'HR_OT_OVER_MONTH', 'HR_OT_OVER_EMP', 'HR_OT_OVER_NIK')`))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard WHERE layout_config ? 'top_section'`))
		require.NoError(t, m.Up(ctx))
		assertTrendFinal(ctx, t, db)
	})

	t.Run("whole HR chain down/up", func(t *testing.T) {
		require.NoError(t, m.DownTo(ctx, preHRWorkforce))
		assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard WHERE dashboard_code LIKE 'HR_%'`))
		assert.False(t, tableExists(ctx, t, db, "bi_hr_stg_ot_daily"))
		require.NoError(t, m.Up(ctx))
		assertTrendFinal(ctx, t, db)
	})
}

func assertWorkforce588(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	assert.Equal(t, 5, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard d JOIN bi_dashboard_group g ON g.group_id = d.group_id
		WHERE g.group_code = 'WORKFORCE'`))
	assert.Equal(t, 10, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard_role r JOIN bi_dashboard d ON d.dashboard_id = r.dashboard_id
		WHERE d.dashboard_code LIKE 'HR_%'`))
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bi_job WHERE job_name = 'ETL_HR_OT_MIDDAY'`))
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bi_job WHERE job_name = 'ETL_HR_OT_DAILY'
		AND schedule_cron = 'CRON_TZ=Asia/Jakarta 0 5 * * *'`))
}

// assertTrendFinal checks HR_OT_TREND and its companion dashboards after the squashed 596/597.
func assertTrendFinal(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	var grain, chartType string
	var drill bool
	var chartCfg, layoutCfg, kpiCfg []byte
	require.NoError(t, db.QueryRowContext(ctx, `SELECT periode_grain, chart_type, drill_enabled, chart_config, layout_config, kpi_config
		FROM bi_dashboard WHERE dashboard_code = 'HR_OT_TREND'`).Scan(&grain, &chartType, &drill, &chartCfg, &layoutCfg, &kpiCfg))
	assert.Equal(t, "MONTHLY", grain)
	assert.Equal(t, "stacked_bar", chartType)
	assert.False(t, drill)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(chartCfg, &cfg))
	assert.Equal(t, "group_2", cfg["stack_by_field"])
	assert.NotEmpty(t, cfg["filter_chips_group1"], "dept chips enabled")

	var layout struct {
		Secondary []struct {
			ChartType string `json:"chart_type"`
		} `json:"secondary_charts"`
	}
	require.NoError(t, json.Unmarshal(layoutCfg, &layout))
	var cards []string
	for _, c := range layout.Secondary {
		cards = append(cards, c.ChartType)
	}
	assert.Equal(t, []string{"ot_mix", "ot_compare", "data_table", "ot_pareto"}, cards)

	var kpis []map[string]any
	require.NoError(t, json.Unmarshal(kpiCfg, &kpis))
	assert.Len(t, kpis, 6)

	// Companion source dashboards exist; removed ones (tier, data quality) never appear.
	for _, code := range []string{"HR_OT_PARETO", "HR_OT_OVER_MONTH", "HR_OT_OVER_EMP", "HR_OT_OVER_NIK"} {
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard WHERE dashboard_code = '`+code+`'`), code)
	}
	assert.Zero(t, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard
		WHERE dashboard_code IN ('HR_OT_TIER_MONTH', 'HR_OT_DQ_MONTH', 'HR_OT_DQ_EMP')`))
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bi_dashboard WHERE dashboard_code = 'HR_OT_OVERLIMIT'
		AND layout_config->'top_section'->>'nik_dashboard' = 'HR_OT_OVER_NIK'`))
}

// hrDashboardSnapshot renders every HR dashboard row (config columns included) as one string.
func hrDashboardSnapshot(ctx context.Context, t *testing.T, db *sql.DB) string {
	t.Helper()
	var snap string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COALESCE(string_agg(row_to_json(x)::text, E'\n' ORDER BY x.dashboard_code), '')
		FROM (SELECT dashboard_code, dashboard_title, periode_grain, chart_type, chart_config, layout_config, kpi_config,
		             drill_enabled, max_drill_level, display_order FROM bi_dashboard WHERE dashboard_code LIKE 'HR_%') x`).Scan(&snap))
	return snap
}

func constraintValidated(ctx context.Context, t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var ok bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT convalidated FROM pg_constraint WHERE conname = $1`, name).Scan(&ok))
	return ok
}
