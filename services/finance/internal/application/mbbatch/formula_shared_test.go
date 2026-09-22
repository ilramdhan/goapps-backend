package mbbatch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// noSharingLoader serves one MB whose single RM is an ITEM-type reference (no nested MB), so
// LoadRMCosts is the ONLY thing that varies per calc type — exactly like the real
// ACTUAL=cost_val / FORECAST=cost_mark / SELLING=cost_sim split in costcalc/loader.go. It
// carries the real 7 F_MB_* formulas (migration 000452_seed_mst_formula_mb.up.sql) so this
// test exercises the exact same formula chain production runs, not a stand-in.
type noSharingLoader struct {
	productID int64
	rmCostFor map[string]float64 // calcType -> RM_COST loader would produce for it
}

func (f *noSharingLoader) LoadProducts(context.Context, []int64) (map[int64]*costproductmaster.CostProductMaster, error) {
	return map[int64]*costproductmaster.CostProductMaster{}, nil
}

func (f *noSharingLoader) LoadRoutesByProducts(_ context.Context, ids []int64) (map[int64]*costroute.Graph, error) {
	out := map[int64]*costroute.Graph{}
	for _, id := range ids {
		rm := &costroute.Rm{
			RmID: 1, SeqID: 1,
			RmType:       costroute.RmTypeItem,
			RmItemCode:   "RM1",
			RouteRmRatio: 1.0,
		}
		out[id] = &costroute.Graph{
			Head: &costroute.Head{HeadID: 901, ProductSysID: id, RoutingStatus: costroute.StatusComplete},
			Seqs: []*costroute.Seq{
				{SeqID: 1, HeadID: 901, ProductSysID: id, RouteLevel: 1, RouteSeq: 1, Rms: []*costroute.Rm{rm}},
			},
		}
	}
	return out, nil
}

// LoadCAPP supplies every non-cost input the 7 MB formulas need, IDENTICAL across calc
// types. This is what makes MB_NET_PROD/MB_FIXED_TOTAL come out equal across ACTUAL/
// FORECAST/SELLING even though each type now computes them independently: the inputs are
// the same, not because the value is copied.
func (f *noSharingLoader) LoadCAPP(_ context.Context, ids []int64) (map[int64]map[string]float64, error) {
	out := map[int64]map[string]float64{}
	for _, id := range ids {
		out[id] = map[string]float64{
			"MB_WASTE":               10,
			"MB_THROUGHPUT":          100,
			"MB_EFFICIENCY":          90,
			"MB_PROD_PER_DAY":        1,
			"MACHINE_MB_FIXED_TOTAL": 900,
			"MB_QUALITY_LOSS":        2,
			"MB_DEV_EXPENSE":         1,
			"MB_PACKING":             5,
			"MB_NO_PROCESS":          1,
			"IS_BOUGHTOUT":           0,
		}
	}
	return out, nil
}

func (f *noSharingLoader) LoadCAPPText(context.Context, []int64) (map[int64]map[string]string, error) {
	return map[int64]map[string]string{}, nil
}

func (f *noSharingLoader) LoadCalculatedParams(context.Context, []int64) (map[int64]map[string]bool, error) {
	return map[int64]map[string]bool{}, nil
}

// LoadFormulas returns the real 7 F_MB_* formulas, topo-sorted exactly as the migration's
// PART 2 formula_param rows drive them in production.
func (f *noSharingLoader) LoadFormulas(_ context.Context, ids []int64) (map[int64][]costcalc.Formula, error) {
	formulas := []costcalc.Formula{
		{
			FormulaCode: FormulaCodeRMCost, FormulaType: costcalc.FormulaTypeRMLookup,
			ResultParamCode: ResultParamRMCost,
		},
		{
			FormulaCode: FormulaCodeWasteVal, FormulaType: "CALCULATION",
			Expression:      "(MB_RM_COST / (1 - MB_WASTE/100)) * (MB_WASTE/100)",
			ResultParamCode: ResultParamWasteVal,
			InputParamCodes: []string{"MB_RM_COST", "MB_WASTE"},
		},
		{
			FormulaCode: FormulaCodeNetProd, FormulaType: "CALCULATION",
			Expression:      "MB_THROUGHPUT * (MB_EFFICIENCY/100) * MB_PROD_PER_DAY",
			ResultParamCode: ResultParamNetProd,
			InputParamCodes: []string{"MB_THROUGHPUT", "MB_EFFICIENCY", "MB_PROD_PER_DAY"},
		},
		{
			FormulaCode: FormulaCodeFixedCost, FormulaType: "CALCULATION",
			Expression:      "MB_NET_PROD > 0 ? MACHINE_MB_FIXED_TOTAL / MB_NET_PROD : 0",
			ResultParamCode: ResultParamFixedCost,
			InputParamCodes: []string{"MB_NET_PROD", "MACHINE_MB_FIXED_TOTAL"},
		},
		{
			FormulaCode: FormulaCodeOthers, FormulaType: "CALCULATION",
			Expression:      "((MB_RM_COST + MB_WASTE_VAL + MB_FIXED_TOTAL) * ((MB_QUALITY_LOSS + MB_DEV_EXPENSE)/100)) + MB_PACKING",
			ResultParamCode: ResultParamOthers,
			InputParamCodes: []string{"MB_RM_COST", "MB_WASTE_VAL", "MB_FIXED_TOTAL", "MB_QUALITY_LOSS", "MB_DEV_EXPENSE", "MB_PACKING"},
		},
		{
			FormulaCode: FormulaCodeConvCost, FormulaType: "CALCULATION",
			Expression:      "(MB_NO_PROCESS * MB_FIXED_TOTAL) + MB_COST_OTHERS + MB_WASTE_VAL",
			ResultParamCode: ResultParamConvCost,
			InputParamCodes: []string{"MB_NO_PROCESS", "MB_FIXED_TOTAL", "MB_COST_OTHERS", "MB_WASTE_VAL"},
		},
		{
			FormulaCode: FormulaCodeFinalCost, FormulaType: "CALCULATION",
			Expression:      "IS_BOUGHTOUT == 1 ? MB_RM_COST : MB_RM_COST + MB_CONV_COST",
			ResultParamCode: ResultParamFinalCost,
			InputParamCodes: []string{"IS_BOUGHTOUT", "MB_RM_COST", "MB_CONV_COST"},
		},
	}
	out := map[int64][]costcalc.Formula{}
	for _, id := range ids {
		out[id] = formulas
	}
	return out, nil
}

// LoadRMCosts is where the calc types diverge: each calc type's RM_COST is deliberately far
// apart (100 / 120 / 80) so any accidental copy from another calc type's pass is obvious in
// the assertions below rather than lost in rounding noise.
//
// RM1 is a GROUP-type RM (service.go resolves groupCodes for LoadRMCosts), so
// resolveRMUnitCost's cascade reads CrRate, not CostVal — both are set to the
// same per-calc-type value here to keep the fixture meaningful.
func (f *noSharingLoader) LoadRMCosts(_ context.Context, _ []string, _ string, calcType string) (map[string]costcalc.RMCostRates, error) {
	v := f.rmCostFor[calcType]
	return map[string]costcalc.RMCostRates{"RM1|": {CostVal: v, CrRate: v}}, nil
}

func (f *noSharingLoader) LoadUpstreamCosts(context.Context, []int64, string, string) (map[int64]float64, error) {
	return map[int64]float64{}, nil
}

func (f *noSharingLoader) LoadSellingSnapshots(context.Context, []int64, string) (map[int64]map[string]float64, error) {
	return map[int64]map[string]float64{}, nil
}

func (f *noSharingLoader) LoadMBCosts(context.Context, []string) (map[string]map[string]float64, error) {
	return map[string]map[string]float64{}, nil
}

func (f *noSharingLoader) LoadSpinFixedCost(context.Context, string) (costcalc.SpinPool, error) {
	return costcalc.SpinPool{}, nil
}

// mbAllSevenFormulaCodes is the ResultParamCode of every one of the 7 F_MB_* formulas, used
// to assert the persisted param snapshot carries all 7 for every calc type, not a subset.
var mbAllSevenFormulaCodes = []string{
	ResultParamRMCost, ResultParamWasteVal, ResultParamNetProd, ResultParamFixedCost,
	ResultParamOthers, ResultParamConvCost, ResultParamFinalCost,
}

// TestComputeAndPersist_AllSevenFormulasIndependentPerCalcType is THE regression test for the
// removed sharing mechanism (sharedFormulaCodes/partitionFormulas/mergeCAPP/sharedOutputs).
//
// It drives Service.computeAndPersist directly (tx is nil — the fakeResultWriter never
// dereferences it) with RM_COST made deliberately different per calc type, and checks:
//
//  1. MB_WASTE_VAL, MB_COST_OTHERS, MB_CONV_COST and MB_FINAL_COST are DIFFERENT across
//     ACTUAL/FORECAST/SELLING, following the exact formula math for each type's own
//     RM_COST — the old bug forced FORECAST/SELLING's values to equal ACTUAL's regardless
//     of their own RM_COST.
//  2. MB_NET_PROD and MB_FIXED_TOTAL come out EQUAL across all three types — not because
//     they are copied, but because their non-cost inputs (MB_THROUGHPUT, MB_EFFICIENCY,
//     MACHINE_MB_FIXED_TOTAL, ...) are identical across types. This is the "same answer,
//     computed independently" case the business explicitly asked to keep computing fresh.
//  3. The persisted cpc_formula_trace (out.FormulaTrace, threaded into the fake
//     resultWriter's captured Result) has exactly 7 entries for EVERY calc type — before
//     this fix FORECAST/SELLING only ran 2 formulas (RM_COST, FINAL_COST), so their trace
//     had only 2 entries even though 5 stale values were merged into their CAPP/snapshot.
//  4. The persisted cpc_param_snapshot (out.ParamSnapshot) contains all 7 MB result param
//     codes for EVERY calc type, proving persistence — not just in-memory computation —
//     carries the full 7-formula result for ACTUAL, FORECAST and SELLING alike.
func TestComputeAndPersist_AllSevenFormulasIndependentPerCalcType(t *testing.T) {
	const productID = int64(70001)
	loader := &noSharingLoader{
		productID: productID,
		rmCostFor: map[string]float64{
			"ACTUAL":   100,
			"FORECAST": 120,
			"SELLING":  80,
		},
	}
	writer := &fakeResultWriter{}
	svc := &Service{
		loader:       loader,
		resultWriter: writer,
		evalCache:    evaluator.NewCache(),
	}
	candidate := MBHeadCandidate{MBHID: "mbh-no-sharing", Code: "MB-NOSHARE", CostProductID: productID}
	costs := newBatchCosts([]MBHeadCandidate{candidate})

	computed, _, err := svc.computeAndPersist(context.Background(), nil, candidate, "202609", 999, costs)
	require.NoError(t, err)
	require.Len(t, computed, 3, "one cost-per-unit entry per calc type")
	require.Len(t, writer.got, 3, "one persisted row per calc type")

	byType := map[costcalcdom.CalculationType]*costcalcdom.Result{}
	for _, r := range writer.got {
		byType[r.CalcType()] = r
	}
	require.Len(t, byType, 3)

	// --- 1 & 2: math per calc type, straight from the formulas above -------------------
	expected := map[costcalcdom.CalculationType]struct {
		wasteVal, netProd, fixedTotal, costOthers, convCost, finalCost float64
	}{}
	for calcType, rmCost := range map[costcalcdom.CalculationType]float64{
		costcalcdom.CalcTypeActual:   100,
		costcalcdom.CalcTypeForecast: 120,
		costcalcdom.CalcTypeSelling:  80,
	} {
		wasteVal := (rmCost / (1 - 10.0/100)) * (10.0 / 100)
		netProd := 100.0 * (90.0 / 100) * 1.0 // = 90, identical across types
		fixedTotal := 900.0 / netProd         // = 10, identical across types
		costOthers := ((rmCost + wasteVal + fixedTotal) * ((2.0 + 1.0) / 100)) + 5
		convCost := (1 * fixedTotal) + costOthers + wasteVal
		finalCost := rmCost + convCost // IS_BOUGHTOUT == 0
		expected[calcType] = struct {
			wasteVal, netProd, fixedTotal, costOthers, convCost, finalCost float64
		}{wasteVal, netProd, fixedTotal, costOthers, convCost, finalCost}
	}

	for calcType, exp := range expected {
		r, ok := byType[calcType]
		require.True(t, ok, "missing persisted row for %s", calcType)

		var snapshot map[string]float64
		require.NoError(t, json.Unmarshal(r.ParamSnapshot(), &snapshot))

		require.InDelta(t, exp.wasteVal, snapshot[ResultParamWasteVal], 1e-6, "%s: MB_WASTE_VAL", calcType)
		require.InDelta(t, exp.netProd, snapshot[ResultParamNetProd], 1e-6, "%s: MB_NET_PROD", calcType)
		require.InDelta(t, exp.fixedTotal, snapshot[ResultParamFixedCost], 1e-6, "%s: MB_FIXED_TOTAL", calcType)
		require.InDelta(t, exp.costOthers, snapshot[ResultParamOthers], 1e-6, "%s: MB_COST_OTHERS", calcType)
		require.InDelta(t, exp.convCost, snapshot[ResultParamConvCost], 1e-6, "%s: MB_CONV_COST", calcType)
		require.InDelta(t, exp.finalCost, snapshot[ResultParamFinalCost], 1e-6, "%s: MB_FINAL_COST", calcType)
		require.InDelta(t, exp.finalCost, r.CostPerUnit(), 1e-6, "%s: persisted CostPerUnit", calcType)

		// --- 4: persisted snapshot carries all 7 formula outputs, for every calc type ---
		for _, code := range mbAllSevenFormulaCodes {
			_, present := snapshot[code]
			require.True(t, present, "%s: param snapshot is missing result param %s", calcType, code)
		}

		// --- 3: persisted formula trace has all 7 entries, for every calc type ---------
		var trace []map[string]any
		require.NoError(t, json.Unmarshal(r.FormulaTrace(), &trace))
		require.Len(t, trace, 7, "%s: cpc_formula_trace must record all 7 F_MB_* formula evaluations, not a subset", calcType)
	}

	// Explicit cross-type divergence check (1): the whole point of removing sharing.
	actual, forecast, selling := byType[costcalcdom.CalcTypeActual], byType[costcalcdom.CalcTypeForecast], byType[costcalcdom.CalcTypeSelling]
	require.NotEqual(t, actual.CostPerUnit(), forecast.CostPerUnit(), "FORECAST final cost must no longer be copied from ACTUAL")
	require.NotEqual(t, actual.CostPerUnit(), selling.CostPerUnit(), "SELLING final cost must no longer be copied from ACTUAL")
	require.NotEqual(t, forecast.CostPerUnit(), selling.CostPerUnit())
}

// TestComputeAndPersist_NetProdAndFixedCostEqualAcrossTypesWhenInputsMatch documents,
// separately from the RM_COST-dependent formulas, that a formula with identical non-cost
// inputs across calc types legitimately produces the same number for every type — this is
// expected and is NOT a sign that sharing crept back in, since each value here is computed
// completely independently within its own calc-type pass (see the main test above for the
// values that must differ).
func TestComputeAndPersist_NetProdAndFixedCostEqualAcrossTypesWhenInputsMatch(t *testing.T) {
	const productID = int64(70002)
	loader := &noSharingLoader{
		productID: productID,
		rmCostFor: map[string]float64{"ACTUAL": 200, "FORECAST": 50, "SELLING": 999},
	}
	writer := &fakeResultWriter{}
	svc := &Service{loader: loader, resultWriter: writer, evalCache: evaluator.NewCache()}
	candidate := MBHeadCandidate{MBHID: "mbh-net-prod", Code: "MB-NETPROD", CostProductID: productID}
	costs := newBatchCosts([]MBHeadCandidate{candidate})

	_, _, err := svc.computeAndPersist(context.Background(), nil, candidate, "202609", 1, costs)
	require.NoError(t, err)
	require.Len(t, writer.got, 3)

	for _, r := range writer.got {
		var snapshot map[string]float64
		require.NoError(t, json.Unmarshal(r.ParamSnapshot(), &snapshot))
		require.InDelta(t, 90.0, snapshot[ResultParamNetProd], 1e-6, "%s: MB_NET_PROD depends only on non-cost inputs", r.CalcType())
		require.InDelta(t, 10.0, snapshot[ResultParamFixedCost], 1e-6, "%s: MB_FIXED_TOTAL depends only on non-cost inputs", r.CalcType())
	}
}
