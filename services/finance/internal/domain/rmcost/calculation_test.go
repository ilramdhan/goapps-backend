package rmcost_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

func mustUUID() uuid.UUID { return uuid.New() }
func nowish() time.Time   { return time.Now() }

// floatEq compares floats with a small epsilon — calculation results involve division
// and multiplication that would otherwise suffer from IEEE-754 rounding mismatches.
const epsilon = 1e-4

func floatEq(t *testing.T, want, got float64, msgAndArgs ...any) {
	t.Helper()
	if math.Abs(want-got) > epsilon {
		assert.InDelta(t, want, got, epsilon, msgAndArgs...)
	}
}

func ptr(v float64) *float64 { return &v }

// ----------------------------------------------------------------------------
// Stage predicate tests
// ----------------------------------------------------------------------------

func TestStage_IsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stage rmcost.Stage
		want  bool
	}{
		{rmcost.StageCons, true},
		{rmcost.StageStores, true},
		{rmcost.StageDept, true},
		{rmcost.StagePO1, true},
		{rmcost.StagePO2, true},
		{rmcost.StagePO3, true},
		{rmcost.StageInit, true},
		{rmcost.Stage("BOGUS"), false},
		{rmcost.Stage(""), false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.stage.IsValid(), "stage=%q", tc.stage)
	}
}

func TestStage_IsInit(t *testing.T) {
	t.Parallel()
	assert.True(t, rmcost.StageInit.IsInit())
	assert.False(t, rmcost.StageCons.IsInit())
}

func TestStageRates_Get(t *testing.T) {
	t.Parallel()
	r := rmcost.StageRates{Cons: 1, Stores: 2, Dept: 3, PO1: 4, PO2: 5, PO3: 6}
	assert.Equal(t, 1.0, r.Get(rmcost.StageCons))
	assert.Equal(t, 2.0, r.Get(rmcost.StageStores))
	assert.Equal(t, 3.0, r.Get(rmcost.StageDept))
	assert.Equal(t, 4.0, r.Get(rmcost.StagePO1))
	assert.Equal(t, 5.0, r.Get(rmcost.StagePO2))
	assert.Equal(t, 6.0, r.Get(rmcost.StagePO3))
	assert.Equal(t, 0.0, r.Get(rmcost.StageInit))
	assert.Equal(t, 0.0, r.Get(rmcost.Stage("BOGUS")))
}

// ----------------------------------------------------------------------------
// AggregateRates tests
// ----------------------------------------------------------------------------

func TestAggregateRates_Empty(t *testing.T) {
	t.Parallel()
	r := rmcost.AggregateRates(nil)
	assert.Equal(t, rmcost.StageRates{}, r)

	r = rmcost.AggregateRates([]rmcost.RateInputs{})
	assert.Equal(t, rmcost.StageRates{}, r)
}

func TestAggregateRates_WorkedExampleFromPlan(t *testing.T) {
	t.Parallel()
	// Item 1: cons(1000, 12,500,000), stores(500, 6,300,000), po1(0, 0)
	// Item 2: cons(800, 9,920,000),  stores(400, 5,000,000), po1(200, 2,500,000)
	items := []rmcost.RateInputs{
		{
			ConsQty:   ptr(1000),
			ConsVal:   ptr(12_500_000),
			StoresQty: ptr(500),
			StoresVal: ptr(6_300_000),
			PO1Qty:    ptr(0),
			PO1Val:    ptr(0),
		},
		{
			ConsQty:   ptr(800),
			ConsVal:   ptr(9_920_000),
			StoresQty: ptr(400),
			StoresVal: ptr(5_000_000),
			PO1Qty:    ptr(200),
			PO1Val:    ptr(2_500_000),
		},
	}
	r := rmcost.AggregateRates(items)

	// 22,420,000 / 1800 = 12,455.555...
	floatEq(t, 12_455.555556, r.Cons, "cons_rate")
	// 11,300,000 / 900 = 12,555.555...
	floatEq(t, 12_555.555556, r.Stores, "stores_rate")
	// 2,500,000 / 200 = 12,500
	floatEq(t, 12_500.0, r.PO1, "po1_rate")
	// everything else has 0 qty → 0 rate
	assert.Equal(t, 0.0, r.Dept)
	assert.Equal(t, 0.0, r.PO2)
	assert.Equal(t, 0.0, r.PO3)
}

func TestAggregateRates_ZeroQtyReturnsZeroRate(t *testing.T) {
	t.Parallel()
	// value but no qty — denominator is zero, rate is zero (not NaN).
	items := []rmcost.RateInputs{
		{ConsQty: ptr(0), ConsVal: ptr(1000)},
	}
	r := rmcost.AggregateRates(items)
	assert.Equal(t, 0.0, r.Cons)
	assert.False(t, math.IsNaN(r.Cons))
	assert.False(t, math.IsInf(r.Cons, 0))
}

func TestAggregateRates_NilPointersTreatedAsZero(t *testing.T) {
	t.Parallel()
	items := []rmcost.RateInputs{
		{ConsQty: ptr(100), ConsVal: ptr(1000)},
		{ /* every pointer nil */ },
		{ConsQty: ptr(100), ConsVal: ptr(1500)},
	}
	r := rmcost.AggregateRates(items)
	// (1000 + 0 + 1500) / (100 + 0 + 100) = 2500/200 = 12.5
	floatEq(t, 12.5, r.Cons)
}

func TestAggregateRates_AllStages(t *testing.T) {
	t.Parallel()
	items := []rmcost.RateInputs{
		{
			ConsQty: ptr(10), ConsVal: ptr(100),
			StoresQty: ptr(5), StoresVal: ptr(25),
			DeptQty: ptr(2), DeptVal: ptr(20),
			PO1Qty: ptr(1), PO1Val: ptr(8),
			PO2Qty: ptr(4), PO2Val: ptr(40),
			PO3Qty: ptr(8), PO3Val: ptr(64),
		},
	}
	r := rmcost.AggregateRates(items)
	floatEq(t, 10.0, r.Cons)
	floatEq(t, 5.0, r.Stores)
	floatEq(t, 10.0, r.Dept)
	floatEq(t, 8.0, r.PO1)
	floatEq(t, 10.0, r.PO2)
	floatEq(t, 8.0, r.PO3)
}

// ----------------------------------------------------------------------------
// SelectRate tests
// ----------------------------------------------------------------------------

func TestSelectRate_FlagResolvesDirectly(t *testing.T) {
	t.Parallel()
	rates := rmcost.StageRates{Cons: 100, Stores: 200, Dept: 300, PO1: 400, PO2: 500, PO3: 600}

	cases := []struct {
		flag     rmcost.Stage
		wantRate float64
		wantUsed rmcost.Stage
	}{
		{rmcost.StageCons, 100, rmcost.StageCons},
		{rmcost.StageStores, 200, rmcost.StageStores},
		{rmcost.StageDept, 300, rmcost.StageDept},
		{rmcost.StagePO1, 400, rmcost.StagePO1},
		{rmcost.StagePO2, 500, rmcost.StagePO2},
		{rmcost.StagePO3, 600, rmcost.StagePO3},
	}
	for _, tc := range cases {
		rate, used := rmcost.SelectRate(rates, tc.flag, nil)
		assert.Equal(t, tc.wantRate, rate, "flag=%q", tc.flag)
		assert.Equal(t, tc.wantUsed, used, "flag=%q", tc.flag)
	}
}

func TestSelectRate_CascadeFromDept(t *testing.T) {
	t.Parallel()
	// Requested DEPT but it's zero → cascade starts from CONS.
	rates := rmcost.StageRates{Cons: 12_455.56, Stores: 12_555.56, Dept: 0}
	rate, used := rmcost.SelectRate(rates, rmcost.StageDept, nil)
	floatEq(t, 12_455.56, rate)
	assert.Equal(t, rmcost.StageCons, used)
}

func TestSelectRate_CascadeSkipsZerosUntilMatch(t *testing.T) {
	t.Parallel()
	// Cons=0, Stores=0, Dept=0, PO1=0, PO2=0, PO3=999 → cascade finds PO3.
	rates := rmcost.StageRates{PO3: 999}
	rate, used := rmcost.SelectRate(rates, rmcost.StageCons, nil)
	assert.Equal(t, 999.0, rate)
	assert.Equal(t, rmcost.StagePO3, used)
}

func TestSelectRate_AllZerosPreservesRequestedFlag(t *testing.T) {
	t.Parallel()
	rate, used := rmcost.SelectRate(rmcost.StageRates{}, rmcost.StagePO2, nil)
	assert.Equal(t, 0.0, rate)
	assert.Equal(t, rmcost.StagePO2, used, "original flag preserved when cascade finds nothing")
}

func TestSelectRate_InitOverride(t *testing.T) {
	t.Parallel()
	// INIT flag + init_val present → returns init_val, no cascade even if other stages > 0.
	rates := rmcost.StageRates{Cons: 999_999}
	initVal := 13_000.0
	rate, used := rmcost.SelectRate(rates, rmcost.StageInit, &initVal)
	assert.Equal(t, 13_000.0, rate)
	assert.Equal(t, rmcost.StageInit, used)
}

func TestSelectRate_InitWithoutInitValReturnsZero(t *testing.T) {
	t.Parallel()
	rates := rmcost.StageRates{Cons: 100}
	rate, used := rmcost.SelectRate(rates, rmcost.StageInit, nil)
	assert.Equal(t, 0.0, rate)
	assert.Equal(t, rmcost.StageInit, used)
}

func TestSelectRate_CascadeOrderIsStable(t *testing.T) {
	t.Parallel()
	// Multiple non-zero stages → cascade picks CONS first even if requested flag was PO_3.
	rates := rmcost.StageRates{Cons: 1, Stores: 2, PO3: 3}
	rate, used := rmcost.SelectRate(rates, rmcost.StagePO2, nil)
	assert.Equal(t, 1.0, rate)
	assert.Equal(t, rmcost.StageCons, used)
}

// ----------------------------------------------------------------------------
// LandedCost tests
// ----------------------------------------------------------------------------

func TestLandedCost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		pct   float64
		rate  float64
		perKg float64
		want  float64
	}{
		{"zero all", 0, 0, 0, 0},
		{"only per-kg", 0, 0, 50, 50},
		{"only pct*rate", 0.2, 100, 0, 20},
		{"plan worked example valuation", 0.20, 12_455.5556, 0.0125, 2_491.1236},
		{"plan INIT example", 0.20, 13_000, 0.0125, 2_600.0125},
		{"all zero edge case", 0.20, 0, 0.0125, 0.0125},
	}
	for _, tc := range cases {
		got := rmcost.LandedCost(tc.pct, tc.rate, tc.perKg)
		floatEq(t, tc.want, got, tc.name)
	}
}

// ----------------------------------------------------------------------------
// CalculateCost (full pipeline) tests
// ----------------------------------------------------------------------------

func TestCalculateCost_WorkedExample(t *testing.T) {
	t.Parallel()
	// Same inputs as TestAggregateRates_WorkedExampleFromPlan with flags per plan §6.
	items := []rmcost.RateInputs{
		{
			ConsQty: ptr(1000), ConsVal: ptr(12_500_000),
			StoresQty: ptr(500), StoresVal: ptr(6_300_000),
			PO1Qty: ptr(0), PO1Val: ptr(0),
		},
		{
			ConsQty: ptr(800), ConsVal: ptr(9_920_000),
			StoresQty: ptr(400), StoresVal: ptr(5_000_000),
			PO1Qty: ptr(200), PO1Val: ptr(2_500_000),
		},
	}
	h := rmcost.HeaderInputs{
		CostPercentage: 0.20,
		CostPerKg:      0.0125,
		FlagValuation:  rmcost.StageCons,
		FlagMarketing:  rmcost.StageStores,
		FlagSimulation: rmcost.StagePO1,
	}
	comp := rmcost.CalculateCost(items, h)

	floatEq(t, 2_491.1236, comp.CostValuation, "cost_val")
	floatEq(t, 2_511.1236, comp.CostMarketing, "cost_mark")
	floatEq(t, 2_500.0125, comp.CostSimulation, "cost_sim")
	assert.Equal(t, rmcost.StageCons, comp.FlagValuationUsed)
	assert.Equal(t, rmcost.StageStores, comp.FlagMarketingUsed)
	assert.Equal(t, rmcost.StagePO1, comp.FlagSimulationUsed)
}

func TestCalculateCost_CascadeExample(t *testing.T) {
	t.Parallel()
	// Valuation requests DEPT but dept=0 → cascades to CONS.
	items := []rmcost.RateInputs{
		{ConsQty: ptr(1800), ConsVal: ptr(22_420_000)},
	}
	h := rmcost.HeaderInputs{
		CostPercentage: 0.20,
		CostPerKg:      0.0125,
		FlagValuation:  rmcost.StageDept,
		FlagMarketing:  rmcost.StageCons,
		FlagSimulation: rmcost.StageCons,
	}
	comp := rmcost.CalculateCost(items, h)

	floatEq(t, 22_420_000.0/1800.0, comp.Rates.Cons)
	assert.Equal(t, rmcost.StageCons, comp.FlagValuationUsed, "cascaded")
	assert.Equal(t, rmcost.StageDept, comp.FlagValuation, "original flag preserved")
}

func TestCalculateCost_AllZeroEdgeCase(t *testing.T) {
	t.Parallel()
	items := []rmcost.RateInputs{
		{ConsQty: ptr(0), ConsVal: ptr(0)},
	}
	h := rmcost.HeaderInputs{
		CostPercentage: 0.20,
		CostPerKg:      0.0125,
		FlagValuation:  rmcost.StageCons,
		FlagMarketing:  rmcost.StageStores,
		FlagSimulation: rmcost.StagePO1,
	}
	comp := rmcost.CalculateCost(items, h)

	// Cost = (0.20 × 0) + 0.0125 = 0.0125 per purpose.
	floatEq(t, 0.0125, comp.CostValuation)
	floatEq(t, 0.0125, comp.CostMarketing)
	floatEq(t, 0.0125, comp.CostSimulation)
	// Original flags are preserved in FlagUsed when cascade found nothing.
	assert.Equal(t, rmcost.StageCons, comp.FlagValuationUsed)
	assert.Equal(t, rmcost.StageStores, comp.FlagMarketingUsed)
	assert.Equal(t, rmcost.StagePO1, comp.FlagSimulationUsed)
}

func TestCalculateCost_InitOverride(t *testing.T) {
	t.Parallel()
	items := []rmcost.RateInputs{
		{ConsQty: ptr(100), ConsVal: ptr(999_999)},
	}
	initVal := 13_000.0
	h := rmcost.HeaderInputs{
		CostPercentage:   0.20,
		CostPerKg:        0.0125,
		FlagValuation:    rmcost.StageInit,
		FlagMarketing:    rmcost.StageCons,
		FlagSimulation:   rmcost.StageCons,
		InitValValuation: &initVal,
	}
	comp := rmcost.CalculateCost(items, h)

	// Valuation must use the override (13,000), not the aggregated cons rate.
	floatEq(t, 2_600.0125, comp.CostValuation)
	assert.Equal(t, rmcost.StageInit, comp.FlagValuationUsed)
	// Marketing and simulation still use CONS as configured.
	assert.Equal(t, rmcost.StageCons, comp.FlagMarketingUsed)
}

func TestCalculateCost_PartialZeroCascadeMixed(t *testing.T) {
	t.Parallel()
	// Cons=0, Stores=50, Dept=0 → both "Cons" and "Dept" requests should cascade to Stores.
	items := []rmcost.RateInputs{
		{StoresQty: ptr(10), StoresVal: ptr(500)},
	}
	h := rmcost.HeaderInputs{
		CostPercentage: 1.0,
		CostPerKg:      0,
		FlagValuation:  rmcost.StageCons,
		FlagMarketing:  rmcost.StageDept,
		FlagSimulation: rmcost.StageStores,
	}
	comp := rmcost.CalculateCost(items, h)

	floatEq(t, 50.0, comp.CostValuation)
	floatEq(t, 50.0, comp.CostMarketing)
	floatEq(t, 50.0, comp.CostSimulation)
	assert.Equal(t, rmcost.StageStores, comp.FlagValuationUsed)
	assert.Equal(t, rmcost.StageStores, comp.FlagMarketingUsed)
	assert.Equal(t, rmcost.StageStores, comp.FlagSimulationUsed)
}

// ----------------------------------------------------------------------------
// Validation + cost entity tests
// ----------------------------------------------------------------------------

func TestValidatePeriod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"202604", false},
		{"202001", false},
		{"202012", false},
		{"20260", true},   // 5 digits
		{"2026041", true}, // 7 digits
		{"202613", true},  // month 13
		{"202600", true},  // month 00
		{"20260A", true},  // non-digit
		{"", true},
	}
	for _, tc := range cases {
		err := rmcost.ValidatePeriod(tc.in)
		if tc.wantErr {
			require.ErrorIs(t, err, rmcost.ErrInvalidPeriod, "in=%q", tc.in)
		} else {
			require.NoError(t, err, "in=%q", tc.in)
		}
	}
}

func TestRMType_IsValid(t *testing.T) {
	t.Parallel()
	assert.True(t, rmcost.RMTypeGroup.IsValid())
	assert.True(t, rmcost.RMTypeItem.IsValid())
	assert.False(t, rmcost.RMType("OTHER").IsValid())
}

func TestHistoryTriggerReason_IsValid(t *testing.T) {
	t.Parallel()
	assert.True(t, rmcost.TriggerOracleSyncChain.IsValid())
	assert.True(t, rmcost.TriggerGroupUpdate.IsValid())
	assert.True(t, rmcost.TriggerDetailChange.IsValid())
	assert.True(t, rmcost.TriggerManualUI.IsValid())
	assert.False(t, rmcost.HistoryTriggerReason("other").IsValid())
}

func TestNewGroupCost_Validations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		period       string
		rmCode       string
		calculatedBy string
		wantErr      error
	}{
		{"valid", "202604", "GRP-CHIPS", "system", nil},
		{"bad period", "XXX", "GRP", "system", rmcost.ErrInvalidPeriod},
		{"empty rm code", "202604", "", "system", rmcost.ErrEmptyRMCode},
		{"empty calculatedBy", "202604", "GRP", "", rmcost.ErrEmptyCalculatedBy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp := rmcost.Computed{FlagValuation: rmcost.StageCons}
			headID := mustUUID()
			_, err := rmcost.NewGroupCost(tc.period, tc.rmCode, headID, "name", "KG", comp, tc.calculatedBy)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCost_ApplyComputed(t *testing.T) {
	t.Parallel()
	comp := rmcost.Computed{
		CostValuation:      100,
		CostMarketing:      200,
		CostSimulation:     300,
		FlagValuation:      rmcost.StageCons,
		FlagMarketing:      rmcost.StageStores,
		FlagSimulation:     rmcost.StagePO1,
		FlagValuationUsed:  rmcost.StageCons,
		FlagMarketingUsed:  rmcost.StageStores,
		FlagSimulationUsed: rmcost.StagePO1,
	}
	cost, err := rmcost.NewGroupCost("202604", "GRP", mustUUID(), "Group Name", "KG", comp, "system")
	require.NoError(t, err)

	// Recalculate with new values.
	newComp := rmcost.Computed{
		CostValuation:      999,
		FlagValuation:      rmcost.StageInit,
		FlagValuationUsed:  rmcost.StageInit,
		FlagMarketing:      rmcost.StageCons,
		FlagMarketingUsed:  rmcost.StageCons,
		FlagSimulation:     rmcost.StageCons,
		FlagSimulationUsed: rmcost.StageCons,
	}
	require.NoError(t, cost.ApplyComputed(newComp, "editor"))

	require.NotNil(t, cost.CostValuation())
	assert.Equal(t, 999.0, *cost.CostValuation())
	assert.Equal(t, rmcost.StageInit, cost.FlagValuation())
	require.NotNil(t, cost.UpdatedBy())
	assert.Equal(t, "editor", *cost.UpdatedBy())
}

func TestCost_ApplyComputed_EmptyBy(t *testing.T) {
	t.Parallel()
	comp := rmcost.Computed{FlagValuation: rmcost.StageCons}
	cost, err := rmcost.NewGroupCost("202604", "GRP", mustUUID(), "Group", "KG", comp, "system")
	require.NoError(t, err)

	err = cost.ApplyComputed(rmcost.Computed{}, "")
	require.ErrorIs(t, err, rmcost.ErrEmptyCalculatedBy)
}

// ----------------------------------------------------------------------------
// ListFilter tests
// ----------------------------------------------------------------------------

func TestListFilter_Validate(t *testing.T) {
	t.Parallel()

	f := rmcost.ListFilter{Page: 0, PageSize: 0}
	f.Validate()
	assert.Equal(t, 1, f.Page)
	assert.Equal(t, 10, f.PageSize)
	assert.Equal(t, "period", f.SortBy)
	assert.Equal(t, "desc", f.SortOrder)

	f = rmcost.ListFilter{PageSize: 500}
	f.Validate()
	assert.Equal(t, 100, f.PageSize)

	f = rmcost.ListFilter{Page: 3, PageSize: 20}
	f.Validate()
	assert.Equal(t, 40, f.Offset())
}

func TestReconstructCost_Getters(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	headID := uuid.New()
	itemCode := "ITEM"
	rates := rmcost.StageRates{Cons: 1, Stores: 2}
	costVal := 100.0
	costMkt := 200.0
	costSim := 300.0
	calcAt := nowish()
	calcBy := "worker"
	createdAt := nowish()
	updAt := nowish()
	updBy := "editor"

	c := rmcost.ReconstructCost(
		id,
		"202604",
		"GRP",
		rmcost.RMTypeGroup,
		&headID,
		&itemCode,
		"Group Name",
		"KG",
		rates,
		&costVal, &costMkt, &costSim,
		rmcost.StageCons, rmcost.StageStores, rmcost.StagePO1,
		rmcost.StageCons, rmcost.StageStores, rmcost.StagePO1,
		&calcAt,
		&calcBy,
		createdAt,
		"system",
		&updAt,
		&updBy,
	)

	assert.Equal(t, id, c.ID())
	assert.Equal(t, "202604", c.Period())
	assert.Equal(t, "GRP", c.RMCode())
	assert.Equal(t, rmcost.RMTypeGroup, c.RMType())
	assert.Equal(t, &headID, c.GroupHeadID())
	assert.Equal(t, &itemCode, c.ItemCode())
	assert.Equal(t, "Group Name", c.RMName())
	assert.Equal(t, "KG", c.UOMCode())
	assert.Equal(t, rates, c.Rates())
	assert.Equal(t, &costVal, c.CostValuation())
	assert.Equal(t, &costMkt, c.CostMarketing())
	assert.Equal(t, &costSim, c.CostSimulation())
	assert.Equal(t, rmcost.StageCons, c.FlagValuation())
	assert.Equal(t, rmcost.StageStores, c.FlagMarketing())
	assert.Equal(t, rmcost.StagePO1, c.FlagSimulation())
	assert.Equal(t, rmcost.StageCons, c.FlagValuationUsed())
	assert.Equal(t, rmcost.StageStores, c.FlagMarketingUsed())
	assert.Equal(t, rmcost.StagePO1, c.FlagSimulationUsed())
	assert.Equal(t, &calcAt, c.CalculatedAt())
	assert.Equal(t, &calcBy, c.CalculatedBy())
	assert.Equal(t, createdAt, c.CreatedAt())
	assert.Equal(t, "system", c.CreatedBy())
	assert.Equal(t, &updAt, c.UpdatedAt())
	assert.Equal(t, &updBy, c.UpdatedBy())
}

func TestStringMethods(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "CONS", rmcost.StageCons.String())
	assert.Equal(t, "GROUP", rmcost.RMTypeGroup.String())
}

func TestNewListAndHistoryFilter(t *testing.T) {
	t.Parallel()

	lf := rmcost.NewListFilter()
	assert.Equal(t, 1, lf.Page)
	assert.Equal(t, 10, lf.PageSize)
	assert.Equal(t, "period", lf.SortBy)
	assert.Equal(t, "desc", lf.SortOrder)

	hf := rmcost.NewHistoryFilter()
	assert.Equal(t, 1, hf.Page)
	assert.Equal(t, 20, hf.PageSize)
}

func TestHistoryFilter_Validate(t *testing.T) {
	t.Parallel()

	f := rmcost.HistoryFilter{}
	f.Validate()
	assert.Equal(t, 1, f.Page)
	assert.Equal(t, 20, f.PageSize)

	f = rmcost.HistoryFilter{PageSize: 500}
	f.Validate()
	assert.Equal(t, 100, f.PageSize)

	f = rmcost.HistoryFilter{Page: 2, PageSize: 10}
	f.Validate()
	assert.Equal(t, 10, f.Offset())
}
