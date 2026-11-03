package postgres_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Text-level guard for 000605 (route total conversion params/formulas/CAPP). No DB needed.
func TestRouteTotalConversion605MigrationText(t *testing.T) {
	up := readMig(t, "000605_route_total_conversion.up.sql")
	down := readMig(t, "000605_route_total_conversion.down.sql")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.NotContains(t, up, "::")
	assert.NotRegexp(t, `(?im)^\s*UPDATE\s`, up, "additive only")
	assert.NotRegexp(t, `(?im)^\s*DELETE\s`, up, "additive only")

	for _, c := range []string{"CAP_CONV_STAGE", "DEL_CONV_STAGE", "UPSTREAM_TOTAL_CAP_CONV", "TOTAL_CAP_CONV", "TOTAL_DEL_CONV"} {
		assert.Contains(t, up, "'"+c+"', ")
		assert.Contains(t, down, "'"+c+"'")
	}
	assert.Equal(t, 5, strings.Count(up, "INSERT INTO mst_parameter"))
	assert.Equal(t, 4, strings.Count(up, "INSERT INTO mst_formula"))
	assert.Equal(t, 10, strings.Count(up, "INSERT INTO formula_param"))

	// UPSTREAM_TOTAL_CAP_CONV is engine-injected: no formula may produce it.
	assert.NotContains(t, up, "p.param_code = 'UPSTREAM_TOTAL_CAP_CONV'\n  AND p.deleted_at IS NULL\n  AND NOT EXISTS")

	for _, e := range []string{
		"CAPTIVE_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS",
		"DELIVERY_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS",
		"CAP_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV",
		"DEL_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV",
	} {
		assert.Contains(t, up, "'"+e+"'")
	}
	assert.Contains(t, up, "'CALCULATION'")
	assert.Contains(t, up, "np.param_code IN ('CAP_CONV_STAGE','DEL_CONV_STAGE','TOTAL_CAP_CONV','TOTAL_DEL_CONV')")
	assert.Contains(t, up, "pt.cpt_type_code <> 'MB'")
	assert.Contains(t, up, "bp.param_code = 'CAPTIVE_COST_QLTY_LOSS'")
	// Backfill is gated on the explicit COST_STAGE_OUT sink so findTerminalFormula can never
	// pick a new conversion formula as a product's final cost.
	assert.Contains(t, up, "sp.param_code = 'COST_STAGE_OUT'")
	assert.Contains(t, up, "ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING")
	assert.Contains(t, up, "'seed_000605'")
	assert.Contains(t, up, "'seed_route_total_conv_000605'")

	// Down touches only marker rows.
	assert.Contains(t, down, "capp_created_by = 'seed_route_total_conv_000605'")
	assert.Equal(t, 3, strings.Count(down, "created_by = 'seed_000605'"))
	assert.Less(t, strings.Index(down, "DELETE FROM cost_product_applicable_param"), strings.Index(down, "DELETE FROM formula_param"))
	assert.Less(t, strings.Index(down, "DELETE FROM formula_param"), strings.Index(down, "UPDATE mst_formula"))
	assert.Less(t, strings.Index(down, "UPDATE mst_formula"), strings.Index(down, "UPDATE mst_parameter"))
}
