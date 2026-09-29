package costcalc

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration000532_MatchesFixtureExpressions keeps the pre-quality-loss
// fixture (capPreQLExpr / delPreQLExpr) linked to what migration 000532
// actually writes (000510 / 000524 test pattern): the file is parsed as text,
// never executed.
func TestMigration000532_MatchesFixtureExpressions(t *testing.T) {
	const up = "../../../migrations/postgres/000532_add_mb_cost_to_pre_qloss_formulas.up.sql"
	const down = "../../../migrations/postgres/000532_add_mb_cost_to_pre_qloss_formulas.down.sql"

	rawUp, err := os.ReadFile(up)
	require.NoError(t, err, "migration 000532 up must exist")
	upSQL := stripSQLComments(string(rawUp))

	for code, expr := range map[string]string{
		"F_YARN_CAP_PRE_QL": capPreQLExpr,
		"F_YARN_DEL_PRE_QL": delPreQLExpr,
	} {
		assert.Contains(t, upSQL, "'"+code+"'", "migration 000532 must name %s", code)
		assert.Contains(t, upSQL, "'"+expr+"'",
			"migration 000532 must set %s to the expression the fixture models", code)
	}

	// Guards: exact 000408 text, MB_COST_MKT existence check, edge insert.
	assert.Contains(t, upSQL, "'RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB'",
		"the UPDATE must be pinned to the exact 000408 captive text")
	assert.Contains(t, upSQL, "'RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB'",
		"the UPDATE must be pinned to the exact 000408 delivery text")
	assert.Contains(t, upSQL, "RAISE EXCEPTION", "a missing MB_COST_MKT param must abort the migration")
	assert.Contains(t, upSQL, "INSERT INTO formula_param", "MB_COST_MKT must be declared as an input edge")
	assert.Contains(t, upSQL, "SELECT f.id, p.id, 4", "the MB_COST_MKT edge continues 000408's sort order at 4")
	assert.Contains(t, upSQL, "NOT EXISTS", "the edge insert must be idempotent")
	assert.Contains(t, upSQL, "'add_mb_cost_000532'", "rewritten rows must carry the marker")
	assert.NotContains(t, upSQL, "- MB_COST_MKT", "MB cost is added, never subtracted")

	rawDown, err := os.ReadFile(down)
	require.NoError(t, err, "migration 000532 down must exist")
	downSQL := stripSQLComments(string(rawDown))
	assert.Contains(t, downSQL, "DELETE FROM formula_param", "down must drop the MB_COST_MKT edges")
	assert.Contains(t, downSQL, "'add_mb_cost_000532'", "down must be keyed on the marker")
	assert.Contains(t, downSQL, "'"+capPreQLExpr+"'", "down must only revert the 000532 captive text")
	assert.Contains(t, downSQL, "'"+delPreQLExpr+"'", "down must only revert the 000532 delivery text")
}

// TestComputeProduct_PreQLossIncludesMBCost proves the 000532 shape adds
// MB_COST_MKT to both pre-quality-loss params exactly once.
func TestComputeProduct_PreQLossIncludesMBCost(t *testing.T) {
	withMB := yarnTerminalInput(true)
	outMB, err := ComputeProduct(context.Background(), withMB)
	require.NoError(t, err)

	noMB := yarnTerminalInput(true)
	noMB.CAPP["MB_COST_MKT"] = 0
	outNoMB, err := ComputeProduct(context.Background(), noMB)
	require.NoError(t, err)

	mb := withMB.CAPP["MB_COST_MKT"]
	require.NotZero(t, mb, "fixture must carry a non-zero MB cost")
	for _, code := range []string{"CAPTIVE_COST_BEFORE_QLOSS", "DELIVERY_COST_BEFORE_QLOSS"} {
		assert.InDelta(t, outNoMB.ParamSnapshot[code]+mb, outMB.ParamSnapshot[code], 1e-9,
			"%s must include MB_COST_MKT exactly once", code)
	}
	assert.InDelta(t, outNoMB.CostPerUnit+mb, outMB.CostPerUnit, 1e-9,
		"MB cost flows once into the delivery terminal")
}
