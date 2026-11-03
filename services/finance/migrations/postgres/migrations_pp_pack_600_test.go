package postgres_test

import (
	"os"
	"regexp"
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
