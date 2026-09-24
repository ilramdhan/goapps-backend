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

// =============================================================================
// Oil cost / oil gain by product type (oil-cost-rm-group, spec §3.5 / §6).
//
// Database-free: every case calls ComputeProduct directly with the formula
// expressions migration 000524 writes (asserted against the migration file by
// TestMigration000524_OilFormulas_MatchesFixtureExpressions below).
// =============================================================================

// oilTol is the numeric tolerance of the spec §6 fixtures.
const oilTol = 1e-9

// Oil RM group codes from decision D1.
const (
	oilGroupConing = "202006101" // PTY
	oilGroupSpin   = "202006077" // POY + Superba
)

// Expressions copied verbatim from 000524_oil_cost_gain_formulas_by_product_type.up.sql.
const (
	oilCostExpr       = "(IS_PTY == 1 || IS_POY == 1) ? ((OPU / (1 - WASTE_PERC / 100)) / (1 - 0.11)) * OIL_RATE / 100 : (IS_SUPERBA == 1 ? (OIL_RATE * OPU) / 1000 : 0)"
	oilGainExpr       = "IS_PTY == 1 ? ((OPU * OIL_RATE) / 100) * -1 : (IS_POY == 1 ? OIL_GAIN_POY_DEFAULT : 0)"
	oilGainPOYDefault = "-0.002"
)

// oilFormulas is the oil sub-chain in topological order (the loader pre-sorts).
// Input edges mirror 000408 + 000524's formula_param rows. poyDefaultExpr lets
// E13 prove the CONSTANT formula is editable without a deploy.
func oilFormulas(poyDefaultExpr string) []Formula {
	return []Formula{
		{
			FormulaCode:     "F_YARN_OIL_GAIN_POY_DEFAULT",
			FormulaName:     "Oil Gain POY Default",
			FormulaType:     "CONSTANT",
			Expression:      poyDefaultExpr,
			ResultParamCode: "OIL_GAIN_POY_DEFAULT",
			SortOrder:       1,
		},
		{
			FormulaCode:     "F_YARN_OIL_COST",
			FormulaName:     "Oil Cost",
			FormulaType:     "CALCULATION",
			Expression:      oilCostExpr,
			ResultParamCode: "OIL_COST",
			InputParamCodes: []string{"OIL_RATE", "OPU", "WASTE_PERC"},
			SortOrder:       2,
		},
		{
			FormulaCode:     "F_YARN_OIL_GAIN",
			FormulaName:     "Oil Gain",
			FormulaType:     "CALCULATION",
			Expression:      oilGainExpr,
			ResultParamCode: "OIL_GAIN",
			InputParamCodes: []string{"OPU", "OIL_RATE", "OIL_GAIN_POY_DEFAULT"},
			SortOrder:       3,
		},
	}
}

// oilCase describes one product for oilInput.
type oilCase struct {
	oil         *OilInput
	opu         float64
	wastePerc   float64
	cappOilRate *float64 // nil = no CAPP OIL_RATE row
	rmCosts     map[string]RMCostRates
	rateOrder   []string
	poyDefault  string
}

// oilInput builds a one-stage ComputeInput whose route consumes an ITEM RM
// (so aggregateRMCost never touches the oil group row) plus the oil chain.
func oilInput(c oilCase) ComputeInput {
	capp := map[string]float64{"OPU": c.opu, "WASTE_PERC": c.wastePerc}
	if c.cappOilRate != nil {
		capp["OIL_RATE"] = *c.cappOilRate
	}
	rm := map[string]RMCostRates{"RM_YARN|": {CostVal: 10}}
	for k, v := range c.rmCosts {
		rm[k] = v
	}
	poy := c.poyDefault
	if poy == "" {
		poy = oilGainPOYDefault
	}
	return ComputeInput{
		ProductSysID: 9101,
		Period:       "202609",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(9101, costroute.RmTypeItem, "RM_YARN", 1.0),
		CAPP:         capp,
		Formulas:     oilFormulas(poy),
		RMCosts:      rm,
		RMRateOrder:  c.rateOrder,
		EvalCache:    evaluator.NewCache(),
		Oil:          c.oil,
	}
}

func ptyOil(groupCode string) *OilInput {
	return &OilInput{
		Class: OilClassPTY, TypeCode: "PTY", GroupCode: groupCode,
		DefaultGroup: oilGroupConing, Allowed: map[string]bool{oilGroupConing: true},
	}
}

func poyOil() *OilInput {
	return &OilInput{
		Class: OilClassPOY, TypeCode: "POY", GroupCode: oilGroupSpin,
		DefaultGroup: oilGroupSpin, Allowed: map[string]bool{oilGroupSpin: true},
	}
}

func superbaOil() *OilInput {
	return &OilInput{
		Class: OilClassSuperba, TypeCode: "TCS", GroupCode: oilGroupSpin,
		DefaultGroup: oilGroupSpin, Allowed: map[string]bool{oilGroupSpin: true},
	}
}

func floatPtr(v float64) *float64 { return &v }

// TestComputeProduct_OilByProductType_Computes covers the success cases of the
// spec §6 table (E1-E5, E7, E10-E13).
func TestComputeProduct_OilByProductType_Computes(t *testing.T) {
	t.Parallel()

	coningRow := map[string]RMCostRates{oilGroupConing + "|": {CrRate: 0, SrRate: 2.2869, PrRate: 3}}
	spinRow := map[string]RMCostRates{oilGroupSpin + "|": {CrRate: 1.5}}

	tests := []struct {
		name         string
		c            oilCase
		wantOilRate  float64
		wantOilCost  float64
		wantOilGain  float64
		wantFlags    [3]float64 // IS_PTY, IS_POY, IS_SUPERBA
		wantRateSkip bool       // OIL_RATE absent from the snapshot
	}{
		{
			// E1: CR=0 so the cascade falls to SR. 2.2/0.993/0.89*2.2869/100.
			name:        "E1 PTY cascades CR->SR",
			c:           oilCase{oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7, rmCosts: coningRow},
			wantOilRate: 2.2869, wantOilCost: 0.0569286126480872, wantOilGain: -0.0503118,
			wantFlags: [3]float64{1, 0, 0},
		},
		{
			// E2: POY shares PTY's OIL_COST arm; OIL_GAIN comes from the constant.
			name:        "E2 POY",
			c:           oilCase{oil: poyOil(), opu: 0.6, wastePerc: 0.7, rmCosts: spinRow},
			wantOilRate: 1.5, wantOilCost: 0.010183645066024, wantOilGain: -0.002,
			wantFlags: [3]float64{0, 1, 0},
		},
		{
			// E3: Superba arm (OIL_RATE*OPU)/1000, OIL_GAIN 0.
			name:        "E3 TCS Superba",
			c:           oilCase{oil: superbaOil(), opu: 0.6, wastePerc: 0.7, rmCosts: spinRow},
			wantOilRate: 1.5, wantOilCost: 0.0009, wantOilGain: 0,
			wantFlags: [3]float64{0, 0, 1},
		},
		{
			// E4: no oil class. OIL_RATE is not resolved (CAPP 5 kept), every
			// flag is 0 so both formulas take their 0 arm, and nothing blocks
			// even though no oil row exists.
			name:        "E4 DTY no oil class is untouched",
			c:           oilCase{oil: nil, opu: 2.2, wastePerc: 0.7, cappOilRate: floatPtr(5)},
			wantOilRate: 5, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{0, 0, 0},
		},
		{
			// E5: the imported CAPP OIL_RATE is ignored for an oil-class product.
			name: "E5 imported CAPP rate is overwritten",
			c: oilCase{
				oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7, cappOilRate: floatPtr(9.99),
				rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CrRate: 2.2869}},
			},
			wantOilRate: 2.2869, wantOilCost: 0.0569286126480872, wantOilGain: -0.0503118,
			wantFlags: [3]float64{1, 0, 0},
		},
		{
			// E7: empty OIL_NAME falls back to the type default.
			name:        "E7 empty OIL_NAME uses the default group",
			c:           oilCase{oil: ptyOil(""), opu: 2.2, wastePerc: 0.7, rmCosts: coningRow},
			wantOilRate: 2.2869, wantOilCost: 0.0569286126480872, wantOilGain: -0.0503118,
			wantFlags: [3]float64{1, 0, 0},
		},
		{
			// E10: a custom RMRateOrder is honored (PR first => 3).
			name: "E10 custom rate order PR,SR,CR",
			c: oilCase{
				oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7, rateOrder: []string{"PR", "SR", "CR"},
				rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CrRate: 1, SrRate: 2, PrRate: 3}},
			},
			wantOilRate: 3, wantOilCost: 0.0746800638175091, wantOilGain: -0.066,
			wantFlags: [3]float64{1, 0, 0},
		},
		{
			// E11: WASTE_PERC = 0 removes the waste gross-up.
			name:        "E11 WASTE_PERC zero",
			c:           oilCase{oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0, rmCosts: coningRow},
			wantOilRate: 2.2869, wantOilCost: 0.0565301123595506, wantOilGain: -0.0503118,
			wantFlags: [3]float64{1, 0, 0},
		},
		{
			// E13: editing F_YARN_OIL_GAIN_POY_DEFAULT changes POY OIL_GAIN.
			name:        "E13 POY default edited to -0.003",
			c:           oilCase{oil: poyOil(), opu: 0.6, wastePerc: 0.7, rmCosts: spinRow, poyDefault: "-0.003"},
			wantOilRate: 1.5, wantOilCost: 0.010183645066024, wantOilGain: -0.003,
			wantFlags: [3]float64{0, 1, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := ComputeProduct(context.Background(), oilInput(tt.c))
			require.NoError(t, err)
			require.NotNil(t, out)
			snap := out.ParamSnapshot

			assert.InDelta(t, tt.wantOilRate, snap[ScopeKeyOilRate], oilTol, "OIL_RATE")
			assert.InDelta(t, tt.wantOilCost, snap["OIL_COST"], oilTol, "OIL_COST")
			assert.InDelta(t, tt.wantOilGain, snap["OIL_GAIN"], oilTol, "OIL_GAIN")

			// E12: the flags land in the snapshot as real floats.
			for i, key := range []string{ScopeKeyIsPTY, ScopeKeyIsPOY, ScopeKeyIsSuperba} {
				v, ok := snap[key]
				require.True(t, ok, "%s must be in the snapshot (not a zero-fill placeholder)", key)
				assert.InDelta(t, tt.wantFlags[i], v, 0, key)
			}
		})
	}
}

// TestComputeProduct_OilRate_Blocks covers the BLOCKED cases (E6, E8, E9 and
// "none configured"): every one must wrap ErrMissingRMCost so the chunk
// processor marks the product BLOCKED / MISSING_RM_COST.
func TestComputeProduct_OilRate_Blocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		c       oilCase
		wantMsg string
	}{
		{
			name:    "E6 missing oil row",
			c:       oilCase{oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7},
			wantMsg: "oil group " + oilGroupConing,
		},
		{
			name: "E8 stored OIL_NAME not allowed for the type",
			c: oilCase{
				oil: ptyOil(oilGroupSpin), opu: 2.2, wastePerc: 0.7,
				rmCosts: map[string]RMCostRates{oilGroupSpin + "|": {CrRate: 1.5}},
			},
			wantMsg: "oil group " + oilGroupSpin + " not allowed for type PTY",
		},
		{
			// D17: an existing row with CR=SR=PR=0 blocks, unlike GROUP RM lines.
			name: "E9 all cascade rates zero",
			c: oilCase{
				oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7,
				rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CrRate: 0, SrRate: 0, PrRate: 0, CostVal: 7}},
			},
			wantMsg: "oil group " + oilGroupConing + ": all rates zero",
		},
		{
			name: "empty OIL_NAME and no default",
			c: oilCase{
				oil:     &OilInput{Class: OilClassPTY, TypeCode: "PTY"},
				opu:     2.2,
				rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CrRate: 1}},
			},
			wantMsg: "oil group: none configured (type PTY)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := ComputeProduct(context.Background(), oilInput(tt.c))
			require.Error(t, err)
			assert.Nil(t, out)
			assert.ErrorIs(t, err, costcalcdom.ErrMissingRMCost)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestResolveOilRate_EmptyAllowedSetSkipsCheck: a type with no allowed-set rows
// only needs the rate row to exist.
func TestResolveOilRate_EmptyAllowedSetSkipsCheck(t *testing.T) {
	t.Parallel()
	in := ComputeInput{
		Oil:     &OilInput{Class: OilClassPOY, TypeCode: "POY", GroupCode: "X1"},
		RMCosts: map[string]RMCostRates{"X1|": {PrRate: 4}},
	}
	rate, label, applied, err := resolveOilRate(in)
	require.NoError(t, err)
	assert.True(t, applied)
	assert.InDelta(t, 4.0, rate, 0)
	assert.Equal(t, "PR", label)
}

// TestInjectProductClassFlags_ExprComparesFloatToIntLiteral (E12): the flags
// are float64 and the stored expressions compare against the int literal 1;
// expr-lang must treat float64(1) == 1 as true.
func TestInjectProductClassFlags_ExprComparesFloatToIntLiteral(t *testing.T) {
	t.Parallel()
	scope := map[string]any{}
	zeroFilled := map[string]bool{ScopeKeyIsPTY: true}
	injectProductClassFlags(scope, zeroFilled, ptyOil(oilGroupConing))

	assert.Empty(t, zeroFilled, "injected flags must leave the zero-filled set")
	assert.IsType(t, float64(0), scope[ScopeKeyIsPTY])

	f := Formula{
		FormulaCode:     "F_TEST_IS_PTY",
		FormulaType:     "CALCULATION",
		Expression:      "IS_PTY == 1 ? 7 : (IS_POY == 1 ? 8 : 9)",
		ResultParamCode: "T_OUT",
	}
	tr, err := evalOneFormula(context.Background(), evaluator.NewCache(), f, scope)
	require.NoError(t, err)
	assert.InDelta(t, 7.0, tr.Output, 0, "float64(1) == 1 must be true in expr-lang")
}

// TestOilGroupCodes_DedupesAndSorts: oil codes join the LoadRMCosts list
// without repeating route codes, and deterministically.
func TestOilGroupCodes_DedupesAndSorts(t *testing.T) {
	t.Parallel()
	oil := map[int64]*OilInput{
		1: {GroupCode: oilGroupConing, DefaultGroup: oilGroupConing},
		2: {GroupCode: "", DefaultGroup: oilGroupSpin},
		3: {GroupCode: " RM_A ", DefaultGroup: oilGroupSpin},
		4: nil,
	}
	got := oilGroupCodes(oil, []string{"RM_A", "RM_B"})
	assert.Equal(t, []string{"RM_A", "RM_B", oilGroupSpin, oilGroupConing}, got)
}

// TestNewOilInput_ParsesAggregates checks the LoadOilContext row mapping.
func TestNewOilInput_ParsesAggregates(t *testing.T) {
	t.Parallel()
	in := newOilInput(" TCS ", "SUPERBA", " 202006077 ", "202006077", "202006077, 202006101,")
	assert.Equal(t, OilClassSuperba, in.Class)
	assert.Equal(t, "TCS", in.TypeCode)
	assert.Equal(t, oilGroupSpin, in.GroupCode)
	assert.Equal(t, oilGroupSpin, in.DefaultGroup)
	assert.Equal(t, map[string]bool{oilGroupSpin: true, oilGroupConing: true}, in.Allowed)

	empty := newOilInput("PTY", "PTY", "", "", "")
	assert.Empty(t, empty.DefaultGroup)
	assert.Empty(t, empty.Allowed)
}

// TestMigration000524_OilFormulas_MatchesFixtureExpressions keeps the fixture
// above linked to what migration 000524 actually writes (000510 test pattern):
// the file is parsed as text, never executed.
func TestMigration000524_OilFormulas_MatchesFixtureExpressions(t *testing.T) {
	const migration = "../../../migrations/postgres/000524_oil_cost_gain_formulas_by_product_type.up.sql"
	raw, err := os.ReadFile(migration)
	require.NoError(t, err, "migration 000524 must exist — the oil tests describe its effect")
	sql := stripSQLComments(string(raw))

	for _, f := range oilFormulas(oilGainPOYDefault) {
		assert.Contains(t, sql, "'"+f.Expression+"'",
			"migration 000524 must write %s's expression exactly as the fixture models it", f.FormulaCode)
		assert.Contains(t, sql, "'"+f.FormulaCode+"'", "migration 000524 must name %s", f.FormulaCode)
	}
	assert.Contains(t, sql, "'CONSTANT'", "F_YARN_OIL_GAIN_POY_DEFAULT must be a CONSTANT formula")

	// Input edges added by 000524 (000408 already supplied OIL_RATE/OPU on
	// F_YARN_OIL_COST). They drive topo order: POY_DEFAULT before OIL_GAIN.
	for _, edge := range []string{
		"('F_YARN_OIL_COST', 'WASTE_PERC',",
		"('F_YARN_OIL_GAIN', 'OPU',",
		"('F_YARN_OIL_GAIN', 'OIL_RATE',",
		"('F_YARN_OIL_GAIN', 'OIL_GAIN_POY_DEFAULT',",
	} {
		assert.Contains(t, sql, edge, "migration 000524 must insert edge %s", edge)
	}
}
