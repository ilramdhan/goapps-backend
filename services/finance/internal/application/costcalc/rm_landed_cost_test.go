package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// rmLandedResultParam is the result param code RM_LANDED_COST is snapshotted
// under (see 000408_seed_oracle_formulas.up.sql line 26).
const rmLandedResultParam = "RM_LANDED_COST"

// landedFormula returns the F_YARN_RM_LANDED formula fixture: RM_LOOKUP type,
// exactly as migration 000519 configures it. evalSingleFormulaStep's
// FormulaTypeRMLookup branch special-cases this FormulaCode to read the
// separately-aggregated landed total instead of aliasing totalRM (see
// compute.go).
func landedFormula() Formula {
	return Formula{
		FormulaCode:     "F_YARN_RM_LANDED",
		FormulaName:     "RM Landed Cost",
		FormulaType:     FormulaTypeRMLookup,
		ResultParamCode: rmLandedResultParam,
	}
}

// TestComputeProduct_GroupRM_Landed_CascadesActualClSlFl covers
// resolveRMLandedCost's GROUP-type cascade for calc_type ACTUAL:
// cl_rate -> sl_rate -> fl_rate, in that fixed default order, independent of
// RM_RATE's own cr_rate/sr_rate/pr_rate cascade (which must keep resolving
// COST_RM_TOTAL/COST_STAGE_OUT exactly as before -- see the final assertion
// in each subtest).
func TestComputeProduct_GroupRM_Landed_CascadesActualClSlFl(t *testing.T) {
	baseIn := func(rates RMCostRates) ComputeInput {
		return ComputeInput{
			ProductSysID: 1,
			Period:       "202604",
			CalcType:     costcalcdom.CalcTypeActual,
			Route:        buildOneStageRoute(1, costroute.RmTypeGroup, "GRP001", 1.0),
			CAPP:         map[string]float64{},
			Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL"), landedFormula()},
			RMCosts:      map[string]RMCostRates{"GRP001|": rates},
			EvalCache:    evaluator.NewCache(),
		}
	}

	t.Run("cl_rate zero, sl_rate positive -> uses sl_rate", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			CrRate: 999.0, // RM_RATE cascade rates -- must not affect landed cost
			SrRate: 999.0,
			PrRate: 999.0,
			ClRate: 0,
			SlRate: 8.5,
			FlRate: 30.0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 8.5, out.ParamSnapshot[rmLandedResultParam], 1e-9)
		// RM_RATE's own cascade (all three rates zero here) must be unaffected.
		assert.InDelta(t, 999.0, out.CostPerUnit, 1e-9)
	})

	t.Run("cl_rate and sl_rate zero, fl_rate positive -> uses fl_rate", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			CrRate: 999.0,
			ClRate: 0,
			SlRate: 0,
			FlRate: 30.0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 30.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
	})

	t.Run("cl_rate positive -> uses cl_rate regardless of sl/fl", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			ClRate: 4.25,
			SlRate: 8.5,
			FlRate: 30.0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 4.25, out.ParamSnapshot[rmLandedResultParam], 1e-9)
	})

	t.Run("all three zero -> zero landed cost, not an error", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			ClRate: 0,
			SlRate: 0,
			FlRate: 0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
	})
}

// TestComputeProduct_GroupRM_Landed_CascadesForecastSpPpFp covers
// resolveRMLandedCost's GROUP-type cascade for calc_type FORECAST:
// sp_rate -> pp_rate -> fp_rate.
func TestComputeProduct_GroupRM_Landed_CascadesForecastSpPpFp(t *testing.T) {
	baseIn := func(rates RMCostRates) ComputeInput {
		return ComputeInput{
			ProductSysID: 1,
			Period:       "202604",
			CalcType:     costcalcdom.CalcTypeForecast,
			Route:        buildOneStageRoute(1, costroute.RmTypeGroup, "GRP001", 1.0),
			CAPP:         map[string]float64{},
			Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL"), landedFormula()},
			RMCosts:      map[string]RMCostRates{"GRP001|": rates},
			EvalCache:    evaluator.NewCache(),
		}
	}

	t.Run("sp_rate zero, pp_rate positive -> uses pp_rate", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			SpRate: 0,
			PpRate: 12.0,
			FpRate: 40.0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 12.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
	})

	t.Run("sp_rate and pp_rate zero, fp_rate positive -> uses fp_rate", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			SpRate: 0,
			PpRate: 0,
			FpRate: 40.0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 40.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
	})

	t.Run("sp_rate positive -> uses sp_rate regardless of pp/fp", func(t *testing.T) {
		out, err := ComputeProduct(context.Background(), baseIn(RMCostRates{
			SpRate: 6.0,
			PpRate: 12.0,
			FpRate: 40.0,
		}))
		require.NoError(t, err)
		assert.InDelta(t, 6.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
	})
}

// TestComputeProduct_GroupRM_Landed_SellingFallsBackToForecast proves the
// deliberate SELLING placeholder: with no F_YARN_RM_LANDED expression config
// supplied (RMLandedOrder nil/empty), calc_type SELLING resolves the SAME
// SP->PP->FP cascade as FORECAST -- per migration 000519's documented,
// intentional decision, not an accident of a missing branch.
func TestComputeProduct_GroupRM_Landed_SellingFallsBackToForecast(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Period:       "202604",
		CalcType:     costcalcdom.CalcTypeSelling,
		Route:        buildOneStageRoute(1, costroute.RmTypeGroup, "GRP001", 1.0),
		CAPP:         map[string]float64{},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL"), landedFormula()},
		RMCosts: map[string]RMCostRates{"GRP001|": {
			SpRate: 0,
			PpRate: 15.0,
			FpRate: 50.0,
		}},
		EvalCache: evaluator.NewCache(),
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.InDelta(t, 15.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
}

// TestComputeProduct_GroupRM_Landed_RespectsLoadedOrder proves a non-default
// cascade order loaded from F_YARN_RM_LANDED's expression (migration 000519,
// costcalc.ParseRMLandedOrder / ComputeInput.RMLandedOrder) is actually
// honored, mirroring TestComputeProduct_GroupRM_RespectsLoadedRMRateOrder for
// RM_RATE.
func TestComputeProduct_GroupRM_Landed_RespectsLoadedOrder(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Period:       "202604",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(1, costroute.RmTypeGroup, "GRP001", 1.0),
		CAPP:         map[string]float64{},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL"), landedFormula()},
		RMCosts: map[string]RMCostRates{"GRP001|": {
			ClRate: 4.25,
			SlRate: 8.5,
			FlRate: 30.0,
		}},
		EvalCache:     evaluator.NewCache(),
		RMLandedOrder: map[string][]string{"ACTUAL": {"FL", "SL", "CL"}},
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	// FL is first in the configured order and is positive -> wins, even
	// though CL (the hardcoded-default first pick) is also positive.
	assert.InDelta(t, 30.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
}

// TestComputeProduct_ProductTypeRM_LandedEqualsRMRate proves the "no
// divergence except GROUP" invariant for PRODUCT-type RMs: RM_LANDED_COST
// and RM_RATE (COST_RM_TOTAL) resolve to the exact same upstream cost, since
// resolveRMLandedCost shares resolveUpstreamProductCost with resolveRMUnitCost.
func TestComputeProduct_ProductTypeRM_LandedEqualsRMRate(t *testing.T) {
	in := ComputeInput{
		ProductSysID:  10,
		Period:        "202604",
		CalcType:      costcalcdom.CalcTypeActual,
		Route:         buildTwoStageRoute(10, 20, 2.0),
		CAPP:          map[string]float64{"WASTE_PCT": 0.0},
		Formulas:      []Formula{finalCostFormula("COST_RM_TOTAL * (1 + WASTE_PCT/100)"), landedFormula()},
		UpstreamCosts: map[int64]float64{20: 50.0},
		EvalCache:     evaluator.NewCache(),
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, out.TotalRMCost, 1e-9) // 50 * 2.0, same as RM_RATE's own test
	assert.InDelta(t, out.TotalRMCost, out.ParamSnapshot[rmLandedResultParam], 1e-9,
		"PRODUCT-type RM: RM_LANDED_COST must equal RM_RATE's total, no distinct landed mechanism")
}

// TestComputeProduct_ItemTypeRM_LandedEqualsRMRate proves the same "no
// divergence except GROUP" invariant for ITEM-type RMs: RM_LANDED_COST and
// RM_RATE both read cost_val, ignoring cr/sr/pr AND cl/sl/fl/sp/pp/fp alike.
func TestComputeProduct_ItemTypeRM_LandedEqualsRMRate(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Period:       "202604",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "ITM001", 1.0),
		CAPP:         map[string]float64{},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL"), landedFormula()},
		RMCosts: map[string]RMCostRates{"ITM001|": {
			CostVal: 42.0,
			CrRate:  100.0, // must be ignored for ITEM-type RMs, both cascades
			SrRate:  100.0,
			PrRate:  100.0,
			ClRate:  100.0,
			SlRate:  100.0,
			FlRate:  100.0,
			SpRate:  100.0,
			PpRate:  100.0,
			FpRate:  100.0,
		}},
		EvalCache: evaluator.NewCache(),
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.InDelta(t, 42.0, out.CostPerUnit, 1e-9)
	assert.InDelta(t, 42.0, out.ParamSnapshot[rmLandedResultParam], 1e-9)
}

// TestComputeProduct_RMRateAliases_UnaffectedByLanded is the regression
// guard: F_YARN_RM_RATE / F_YARN_CAP_CONVERSION / F_YARN_DEL_CONVERSION must
// keep aliasing totalRM exactly as before, even when F_YARN_RM_LANDED is
// present in the same product's formula set and resolves to a DIFFERENT
// value -- proving evalSingleFormulaStep's new branch is scoped to
// F_YARN_RM_LANDED only, not accidentally applied to (or interfering with)
// the other three RM_LOOKUP aliases.
func TestComputeProduct_RMRateAliases_UnaffectedByLanded(t *testing.T) {
	rmRateAlias := Formula{
		FormulaCode:     "F_YARN_RM_RATE",
		FormulaType:     FormulaTypeRMLookup,
		ResultParamCode: "RM_RATE",
	}
	capConversionAlias := Formula{
		FormulaCode:     "F_YARN_CAP_CONVERSION",
		FormulaType:     FormulaTypeRMLookup,
		ResultParamCode: "CAP_CONVERSION",
	}
	in := ComputeInput{
		ProductSysID: 1,
		Period:       "202604",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(1, costroute.RmTypeGroup, "GRP001", 1.0),
		CAPP:         map[string]float64{},
		Formulas: []Formula{
			finalCostFormula("COST_RM_TOTAL"),
			rmRateAlias,
			capConversionAlias,
			landedFormula(),
		},
		RMCosts: map[string]RMCostRates{"GRP001|": {
			CrRate: 3.25, // RM_RATE cascade winner
			ClRate: 4.25, // RM_LANDED cascade winner -- deliberately different
		}},
		EvalCache: evaluator.NewCache(),
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	// Both totalRM-aliased formulas must still equal totalRM (3.25), unaffected
	// by the separately-computed landed total (4.25).
	assert.InDelta(t, 3.25, out.ParamSnapshot["RM_RATE"], 1e-9)
	assert.InDelta(t, 3.25, out.ParamSnapshot["CAP_CONVERSION"], 1e-9)
	assert.InDelta(t, 3.25, out.CostPerUnit, 1e-9)
	// The landed formula resolves independently to its own cascade winner.
	assert.InDelta(t, 4.25, out.ParamSnapshot[rmLandedResultParam], 1e-9)
}
