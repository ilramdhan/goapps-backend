package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readMig(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

// Text-level guard for P002 migrations 000600-000602 (no DB needed).
func TestPackProdStaging600MigrationText(t *testing.T) {
	up := readMig(t, "000600_create_bi_pp_stg_pack_prod_daily.up.sql")
	down := readMig(t, "000600_create_bi_pp_stg_pack_prod_daily.down.sql")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.Contains(t, up, "stg_id               BIGSERIAL      PRIMARY KEY")
	assert.Contains(t, up, "CREATE TABLE IF NOT EXISTS bi_pp_stg_pack_prod_daily")
	for _, c := range []string{"ax_qty", "fq_qty", "dg_qty", "jlt_qty", "total_qty"} {
		assert.Regexp(t, regexp.MustCompile(c+`\s+NUMERIC\(12,6\)\s+NOT NULL DEFAULT 0`), up)
	}
	assert.NotRegexp(t, `(?i)\b(float|double|real)\b`, up)
	for _, c := range []string{"pack_date", "week_start", "month_start"} {
		assert.Contains(t, up, "("+c+")")
	}
	assert.Contains(t, down, "DROP TABLE IF EXISTS bi_pp_stg_pack_prod_daily")
}

func TestPackProdRegistry601MigrationText(t *testing.T) {
	up := readMig(t, "000601_seed_bi_metric_registry_pack.up.sql")
	down := readMig(t, "000601_seed_bi_metric_registry_pack.down.sql")
	for _, m := range []string{"PACK_TOTAL", "PACK_AX", "PACK_FQ", "PACK_DG", "PACK_JLT"} {
		assert.Contains(t, up, "'"+m+"'")
		assert.Contains(t, down, "'"+m+"'")
	}
	assert.Contains(t, up, "ON CONFLICT (metric_name) DO NOTHING")
}

func TestPackProdJob602MigrationText(t *testing.T) {
	up := readMig(t, "000602_seed_bi_job_etl_pp_pack_daily.up.sql")
	down := readMig(t, "000602_seed_bi_job_etl_pp_pack_daily.down.sql")
	assert.Contains(t, up, "source_code = 'HRIS_ORACLE'")
	assert.Contains(t, up, "'ETL_PP_PACK_DAILY'")
	assert.Contains(t, up, "'PP_PACK'")
	assert.Contains(t, up, "'0 7 * * *'") // PP-03: 07:00 WIB
	assert.Contains(t, up, "ON CONFLICT (job_name) DO NOTHING")
	assert.Contains(t, down, "job_name = 'ETL_PP_PACK_DAILY'")
}

// P006: seed dashboard PP_PACK_* (000603).
func TestPackProdDashboards603MigrationText(t *testing.T) {
	up := readMig(t, "000603_seed_bi_pp_pack_dashboards.up.sql")
	down := readMig(t, "000603_seed_bi_pp_pack_dashboards.down.sql")
	for _, c := range []string{"PP_PACK_SUMMARY", "PP_PACK_PRODUCT", "PP_PACK_MACHINE", "PP_PACK_POLY"} {
		assert.Contains(t, up, "'"+c+"'")
		assert.Contains(t, down, "'"+c+"'")
	}
	assert.Contains(t, up, `"group":"PACKPROD"`)
	assert.Contains(t, up, `"numerator_metric":"PACK_AX"`)
	assert.Contains(t, up, `"numerator_metric":"PACK_FQ"`)
	assert.Contains(t, up, `"category_field":"dimension_key"`)
	assert.Contains(t, up, "ON CONFLICT (dashboard_code) DO NOTHING")
	assert.Contains(t, up, "MV_REFRESH_PP_PACK")
	assert.Contains(t, down, "MV_REFRESH_PP_PACK")
}

// P011: jadwal WIB (000604).
func TestPackProdSchedule604MigrationText(t *testing.T) {
	up := readMig(t, "000604_fix_pp_pack_job_schedule_wib.up.sql")
	down := readMig(t, "000604_fix_pp_pack_job_schedule_wib.down.sql")
	assert.Contains(t, up, "'CRON_TZ=Asia/Jakarta 0 7 * * *'")
	assert.Contains(t, up, "'CRON_TZ=Asia/Jakarta 15 7 * * *'")
	assert.Contains(t, up, "RAISE EXCEPTION")
	assert.Contains(t, down, "schedule_cron = '0 7 * * *'")
	assert.Contains(t, down, "schedule_cron = '15 7 * * *'")
}

// P014 / PP-15: seed ulang dashboard di grup PRODUCTION (000606).
func TestPackProdDashboards606MigrationText(t *testing.T) {
	up := readMig(t, "000606_reseed_bi_pp_pack_dashboards_production.up.sql")
	down := readMig(t, "000606_reseed_bi_pp_pack_dashboards_production.down.sql")
	for _, c := range []string{"PP_PACK_SUMMARY", "PP_PACK_PRODUCT", "PP_PACK_MACHINE", "PP_PACK_POLY"} {
		assert.Contains(t, up, "'"+c+"'")
		assert.Contains(t, down, "'"+c+"'")
	}
	assert.Contains(t, up, "WHEN 'PRODUCTION' THEN 1")
	assert.Contains(t, up, "ON CONFLICT (dashboard_code) DO NOTHING")
	assert.Len(t, regexp.MustCompile(`RAISE EXCEPTION`).FindAllString(up, -1), 2)
	assert.Equal(t, 4, strings.Count(up, `"seeded_by":"000606"`))
	assert.Contains(t, down, "layout_config->>'seeded_by' = '000606'")

	// Isi dashboard wajib identik dengan 000603 selain penanda seeded_by.
	block := func(s string) string {
		i := strings.Index(s, "INSERT INTO bi_dashboard (")
		j := strings.Index(s, "ON CONFLICT (dashboard_code) DO NOTHING;")
		require.True(t, i >= 0 && j > i)
		return s[i:j]
	}
	seed603 := readMig(t, "000603_seed_bi_pp_pack_dashboards.up.sql")
	assert.Equal(t, block(seed603), strings.ReplaceAll(block(up), `,"seeded_by":"000606"`, ""))
}
