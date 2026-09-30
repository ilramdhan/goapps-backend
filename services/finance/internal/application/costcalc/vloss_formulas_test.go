package costcalc

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// V-loss expressions exactly as migration 000546 writes them (design D1/D3).
// Asserted against the migration file by TestMigration000546_MatchesFixtureExpressions.
const (
	nonStdVLossExpr = "(AE_PERC + A9_PERC + A_PERC) / 100.0 * NS_LOSS"
	bcVLossCapExpr  = "(CAPTIVE_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0"
	bcVLossDelExpr  = "(DELIVERY_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0"
)

// vLossFormulas is the 000546 quality-loss sub-chain: the three rewritten
// V-loss formulas plus their 000408 consumers (QLOSS_* and *_FINAL). The
// pre-quality-loss costs arrive as CAPP leaves.
func vLossFormulas() []Formula {
	return []Formula{
		{
			FormulaCode: "F_YARN_NON_STD_LOSS", FormulaType: "CALCULATION",
			Expression: nonStdVLossExpr, ResultParamCode: "NON_STD_VALUE_LOSS",
			InputParamCodes: []string{"AE_PERC", "A9_PERC", "A_PERC", "NS_LOSS"}, SortOrder: 0,
		},
		{
			FormulaCode: "F_YARN_BC_LOSS_CAP", FormulaType: "CALCULATION",
			Expression: bcVLossCapExpr, ResultParamCode: "BC_VAL_LOSS_CAPTIVE",
			InputParamCodes: []string{"CAPTIVE_COST_BEFORE_QLOSS", "STD_SP_BC", "B_PERC", "C_PERC"}, SortOrder: 1,
		},
		{
			FormulaCode: "F_YARN_BC_LOSS_DEL", FormulaType: "CALCULATION",
			Expression: bcVLossDelExpr, ResultParamCode: "BC_VAL_LOSS_DELIVERY",
			InputParamCodes: []string{"DELIVERY_COST_BEFORE_QLOSS", "STD_SP_BC", "B_PERC", "C_PERC"}, SortOrder: 2,
		},
		{
			FormulaCode: "F_YARN_QLOSS_CAP", FormulaType: "CALCULATION",
			Expression: "BC_VAL_LOSS_CAPTIVE + NON_STD_VALUE_LOSS", ResultParamCode: "QLTY_LOSS_CAPTIVE_COST",
			InputParamCodes: []string{"BC_VAL_LOSS_CAPTIVE", "NON_STD_VALUE_LOSS"}, SortOrder: 3,
		},
		{
			FormulaCode: "F_YARN_QLOSS_DEL", FormulaType: "CALCULATION",
			Expression: "BC_VAL_LOSS_DELIVERY + NON_STD_VALUE_LOSS", ResultParamCode: "QLTY_LOSS_DELIVERY_COST",
			InputParamCodes: []string{"BC_VAL_LOSS_DELIVERY", "NON_STD_VALUE_LOSS"}, SortOrder: 4,
		},
		{
			FormulaCode: "F_YARN_DEL_FINAL", FormulaType: "CALCULATION",
			Expression: "DELIVERY_COST_BEFORE_QLOSS + QLTY_LOSS_DELIVERY_COST", ResultParamCode: "DELIVERY_COST_QLTY_LOSS",
			InputParamCodes: []string{"DELIVERY_COST_BEFORE_QLOSS", "QLTY_LOSS_DELIVERY_COST"}, SortOrder: 5,
		},
	}
}

func vLossInput(capp map[string]float64) ComputeInput {
	return ComputeInput{
		ProductSysID: 7546,
		Period:       "202609",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(7546, costroute.RmTypeItem, "RM_YARN", 1.0),
		CAPP:         capp,
		Formulas:     vLossFormulas(),
		RMCosts:      map[string]RMCostRates{"RM_YARN|": {CostVal: 1.0}},
		EvalCache:    evaluator.NewCache(),
	}
}

// TestComputeProduct_VLoss_PTYTemplateSample reproduces the costing template's
// PTY sample (design D1): B = 2, C = 0.5, STD_SP_BC = 1.1,
// DELIVERY_COST_BEFORE_QLOSS = 2.014 -> BC V-loss (Del) = (2.014-1.1)*2.5/100
// = 0.02285, which the template prints rounded as 0.023.
func TestComputeProduct_VLoss_PTYTemplateSample(t *testing.T) {
	out, err := ComputeProduct(context.Background(), vLossInput(map[string]float64{
		"CAPTIVE_COST_BEFORE_QLOSS":  1.9,
		"DELIVERY_COST_BEFORE_QLOSS": 2.014,
		"STD_SP_BC":                  1.1,
		"B_PERC":                     2.0,
		"C_PERC":                     0.5,
		"AE_PERC":                    1.0,
		"A9_PERC":                    0.5,
		"A_PERC":                     0.5,
		"NS_LOSS":                    0.05,
	}))
	require.NoError(t, err)

	snap := out.ParamSnapshot
	assert.InDelta(t, 0.02285, snap["BC_VAL_LOSS_DELIVERY"], 1e-12)
	assert.InDelta(t, 0.023, snap["BC_VAL_LOSS_DELIVERY"], 0.0005, "template shows 0.023")
	assert.InDelta(t, (1.9-1.1)*2.5/100, snap["BC_VAL_LOSS_CAPTIVE"], 1e-12)
	assert.InDelta(t, 2.0/100*0.05, snap["NON_STD_VALUE_LOSS"], 1e-12)
	assert.InDelta(t, 0.02285+0.001, snap["QLTY_LOSS_DELIVERY_COST"], 1e-12,
		"quality loss = BC V-loss + NS V-loss (000408 F_YARN_QLOSS_DEL unchanged)")
	assert.InDelta(t, 2.014+0.02385, snap["DELIVERY_COST_QLTY_LOSS"], 1e-12)
}

// TestComputeProduct_VLoss_MissingInputsZeroFill: a product checklisted for
// the V-loss results but with no grade percentages / grade SP must compute 0
// NS loss and not fail — the engine zero-fills the declared inputs.
func TestComputeProduct_VLoss_MissingInputsZeroFill(t *testing.T) {
	out, err := ComputeProduct(context.Background(), vLossInput(map[string]float64{
		"CAPTIVE_COST_BEFORE_QLOSS":  1.9,
		"DELIVERY_COST_BEFORE_QLOSS": 2.014,
	}))
	require.NoError(t, err)
	assert.InDelta(t, 0.0, out.ParamSnapshot["NON_STD_VALUE_LOSS"], 1e-12)
	assert.InDelta(t, 0.0, out.ParamSnapshot["BC_VAL_LOSS_DELIVERY"], 1e-12, "B+C = 0 -> no BC loss")
}

// TestMigration000546_MatchesFixtureExpressions links the fixture above to
// what migration 000546 writes (000532 test pattern; text only, never run).
func TestMigration000546_MatchesFixtureExpressions(t *testing.T) {
	const up = "../../../migrations/postgres/000546_loss_param_dedup_vloss.up.sql"
	const down = "../../../migrations/postgres/000546_loss_param_dedup_vloss.down.sql"

	rawUp, err := os.ReadFile(up)
	require.NoError(t, err, "migration 000546 up must exist")
	upSQL := stripSQLComments(string(rawUp))

	for code, expr := range map[string]string{
		"F_YARN_NON_STD_LOSS": nonStdVLossExpr,
		"F_YARN_BC_LOSS_CAP":  bcVLossCapExpr,
		"F_YARN_BC_LOSS_DEL":  bcVLossDelExpr,
	} {
		assert.Contains(t, upSQL, "WHEN '"+code+"'")
		assert.Contains(t, upSQL, "'"+expr+"'", "000546 must write the %s expression the fixture models", code)
	}
	// Edges declared for every input the fixture models.
	for _, f := range vLossFormulas()[:3] {
		for _, in := range f.InputParamCodes {
			assert.Contains(t, upSQL, "('"+f.FormulaCode+"',", "edge list names %s", f.FormulaCode)
			assert.Contains(t, upSQL, "'"+in+"'", "edge input %s for %s", in, f.FormulaCode)
		}
	}
	// Guards and reversibility.
	assert.Contains(t, upSQL, "'CAPTIVE_COST_BEFORE_QLOSS * (NON_STD_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)'",
		"the UPDATE must be pinned to the exact 000408 text")
	assert.Contains(t, upSQL, "RAISE EXCEPTION", "a missing param must abort")
	assert.Contains(t, upSQL, "bak_loss_param_000546")
	assert.Contains(t, upSQL, "is_active = FALSE")

	rawDown, err := os.ReadFile(down)
	require.NoError(t, err, "migration 000546 down must exist")
	downSQL := stripSQLComments(string(rawDown))
	assert.Contains(t, downSQL, "'loss_param_dedup_000546'")
	assert.Contains(t, downSQL, "bak_loss_param_000546")
	assert.Contains(t, downSQL, "'DELIVERY_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)'",
		"down must restore the 000408 text")
	assert.Contains(t, downSQL, "is_active = TRUE")
}
