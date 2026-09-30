package costcalc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMigration000535_GradeWeightsCalculated asserts 000535 flips only the five
// grade weights (not AX_WT / NET_BOB_WT) from INPUT to CALCULATED, never
// touches stored CPP values, and restores the backup exactly on down.
func TestMigration000535_GradeWeightsCalculated(t *testing.T) {
	up, down := readMigrationPair(t, "000535_grade_weight_params_calculated")
	const codes = "param_code IN ('AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT')"

	assert.Contains(t, up, "UPDATE mst_parameter")
	assert.Contains(t, up, "SET param_category = 'CALCULATED'")
	assert.Contains(t, up, "WHERE "+codes+"\n  AND param_category = 'INPUT'")
	assert.Contains(t, up, "updated_by     = 'grade_wt_calc_000535'")
	assert.Contains(t, up, "INSERT INTO bak_grade_wt_params_000535")
	assert.Contains(t, up, "ON CONFLICT (param_code) DO NOTHING")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.NotContains(t, up, "'NET_BOB_WT'")
	assert.NotContains(t, up, "cost_product_parameter", "stored CPP values are kept")
	assert.NotContains(t, up, "DELETE")
	assert.NotContains(t, up, "param_code = 'AX_WT'\n      AND deleted_at IS NULL\n      AND param_category = 'CALCULATED'")

	assert.Contains(t, down, "SET param_category = b.param_category")
	assert.Contains(t, down, "updated_at     = b.updated_at")
	assert.Contains(t, down, "updated_by     = b.updated_by")
	assert.Contains(t, down, "FROM bak_grade_wt_params_000535 b")
	assert.Contains(t, down, "AND p.updated_by = 'grade_wt_calc_000535'")
	assert.Contains(t, down, "DROP TABLE IF EXISTS bak_grade_wt_params_000535")
	for _, s := range []string{up, down} {
		assert.Contains(t, s, "BEGIN;")
		assert.Contains(t, s, "COMMIT;")
	}
}
