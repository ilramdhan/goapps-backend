package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Text-level guard for 000615 (fixed cost min threshold). No DB needed.
func TestFixcostMinThreshold615MigrationText(t *testing.T) {
	up := readMig(t, "000615_fixcost_min_threshold.up.sql")
	down := readMig(t, "000615_fixcost_min_threshold.down.sql")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	for _, s := range []string{
		"ADD COLUMN IF NOT EXISTS cpm_fixcost_min_threshold NUMERIC(20,6) NULL",
		"ADD COLUMN IF NOT EXISTS mc_fixcost_min_threshold NUMERIC(20,6) NULL",
		"chk_cpm_fixcost_min_threshold", "chk_mc_fixcost_min_threshold",
		">= 0", "FIXEDCOST_SHORTFALL", "'Fixed Cost', 92",
		"'ply'", "'ply ( carp)'", "'ply 30-poss'", "'tfo (lumino)'", "'50% poss tfo (lumino)'",
		"WHERE NOT EXISTS", "fixcost_min_threshold_000615",
	} {
		assert.Contains(t, up, s)
	}
	assert.NotRegexp(t, `(?i)INSERT\s+INTO\s+(cost_product_applicable_param|mst_formula|mst_formula_param)`, up)
	assert.NotRegexp(t, `(?i)\b(GRANT|OWNER TO|search_path)\b`, up)

	for _, s := range []string{
		"DELETE FROM mst_parameter", "created_by = 'fixcost_min_threshold_000615'",
		"DROP CONSTRAINT IF EXISTS chk_cpm_fixcost_min_threshold",
		"DROP CONSTRAINT IF EXISTS chk_mc_fixcost_min_threshold",
		"DROP COLUMN IF EXISTS cpm_fixcost_min_threshold",
		"DROP COLUMN IF EXISTS mc_fixcost_min_threshold",
	} {
		assert.Contains(t, down, s)
	}
}
