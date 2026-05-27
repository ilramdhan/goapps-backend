// Package rmcost provides the landed-cost calculation engine and persistence contract
// for the RM cost aggregates produced from grouped raw-material consumption data.
package rmcost

// Stage selects which per-stage rate feeds the landed-cost formula.
// Mirrors the `rmgroup.Flag` string values exactly so the two types can
// be converted with a single cast at the application-layer boundary.
type Stage string

// Stage constants — MUST match rmgroup.Flag values and the DB CHECK constraints.
const (
	// StageCons selects the consumption-aggregated rate.
	StageCons Stage = "CONS"
	// StageStores selects the stores-aggregated rate.
	StageStores Stage = "STORES"
	// StageDept selects the department-aggregated rate.
	StageDept Stage = "DEPT"
	// StagePO1 selects the purchase-order slot 1 rate.
	StagePO1 Stage = "PO_1"
	// StagePO2 selects the purchase-order slot 2 rate.
	StagePO2 Stage = "PO_2"
	// StagePO3 selects the purchase-order slot 3 rate.
	StagePO3 Stage = "PO_3"
	// StageInit signals that the init_val override should be used instead of any aggregated rate.
	StageInit Stage = "INIT"
)

// IsValid reports whether the stage is one of the recognized values.
func (s Stage) IsValid() bool {
	switch s {
	case StageCons, StageStores, StageDept, StagePO1, StagePO2, StagePO3, StageInit:
		return true
	default:
		return false
	}
}

// IsInit reports whether the stage is the INIT override.
func (s Stage) IsInit() bool { return s == StageInit }

// String returns the canonical string form.
func (s Stage) String() string { return string(s) }

// cascadeOrder is the fixed fallback chain used when the requested stage's rate is zero.
// INIT is intentionally excluded — an INIT request never cascades.
var cascadeOrder = []Stage{StageCons, StageStores, StageDept, StagePO1, StagePO2, StagePO3}

// StageRates holds the aggregated (SUM(val) / SUM(qty)) rate per stage for a group
// in a given period. A zero value means either no data or a denominator of zero.
type StageRates struct {
	Cons   float64
	Stores float64
	Dept   float64
	PO1    float64
	PO2    float64
	PO3    float64
}

// Get returns the rate for the given stage, or 0 when the stage is not a per-stage
// slot (e.g. StageInit, or an unknown value).
func (r StageRates) Get(s Stage) float64 {
	switch s {
	case StageCons:
		return r.Cons
	case StageStores:
		return r.Stores
	case StageDept:
		return r.Dept
	case StagePO1:
		return r.PO1
	case StagePO2:
		return r.PO2
	case StagePO3:
		return r.PO3
	case StageInit:
		return 0
	default:
		return 0
	}
}

// RateInputs are the per-stage numerator/denominator pairs the engine aggregates.
// One RateInputs row corresponds to one `cst_item_cons_stk_po` record for the group's
// period; nil pointers are treated as zero contribution.
type RateInputs struct {
	ConsQty   *float64
	ConsVal   *float64
	StoresQty *float64
	StoresVal *float64
	DeptQty   *float64
	DeptVal   *float64
	PO1Qty    *float64
	PO1Val    *float64
	PO2Qty    *float64
	PO2Val    *float64
	PO3Qty    *float64
	PO3Val    *float64
}

// AggregateRates computes SUM(val) / SUM(qty) per stage across the supplied items.
// When SUM(qty) is zero for a stage, that stage's rate is returned as zero rather
// than producing NaN — the cascade logic in SelectRate treats zero as "no data".
func AggregateRates(items []RateInputs) StageRates {
	var (
		qtyC, valC float64
		qtyS, valS float64
		qtyD, valD float64
		qty1, val1 float64
		qty2, val2 float64
		qty3, val3 float64
	)
	for i := range items {
		it := items[i]
		qtyC += deref(it.ConsQty)
		valC += deref(it.ConsVal)
		qtyS += deref(it.StoresQty)
		valS += deref(it.StoresVal)
		qtyD += deref(it.DeptQty)
		valD += deref(it.DeptVal)
		qty1 += deref(it.PO1Qty)
		val1 += deref(it.PO1Val)
		qty2 += deref(it.PO2Qty)
		val2 += deref(it.PO2Val)
		qty3 += deref(it.PO3Qty)
		val3 += deref(it.PO3Val)
	}
	return StageRates{
		Cons:   safeDiv(valC, qtyC),
		Stores: safeDiv(valS, qtyS),
		Dept:   safeDiv(valD, qtyD),
		PO1:    safeDiv(val1, qty1),
		PO2:    safeDiv(val2, qty2),
		PO3:    safeDiv(val3, qty3),
	}
}

// SelectRate applies the flag + cascade rule.
//
// Returns (selectedRate, stageUsed):
//   - If flag == StageInit: returns (*initVal, StageInit). When initVal is nil, returns (0, StageInit).
//   - Otherwise returns the rate for `flag` if it is > 0.
//   - If the requested rate is zero, cascades in fixed order CONS → STORES → DEPT → PO_1 → PO_2 → PO_3
//     and returns the first non-zero rate with that stage.
//   - If every stage in the cascade is zero, returns (0, flag) — the original flag is preserved
//     in `stageUsed` so callers can record "cascaded but nothing found".
func SelectRate(rates StageRates, flag Stage, initVal *float64) (float64, Stage) {
	if flag == StageInit {
		if initVal != nil {
			return *initVal, StageInit
		}
		return 0, StageInit
	}
	if r := rates.Get(flag); r > 0 {
		return r, flag
	}
	for _, s := range cascadeOrder {
		if r := rates.Get(s); r > 0 {
			return r, s
		}
	}
	return 0, flag
}

// LandedCost = (cost_percentage × selected_rate) + cost_per_kg.
func LandedCost(costPercentage, selectedRate, costPerKg float64) float64 {
	return (costPercentage * selectedRate) + costPerKg
}

// safeDiv returns num/denom, or 0 when denom is zero.
func safeDiv(num, denom float64) float64 {
	if denom == 0 {
		return 0
	}
	return num / denom
}

// deref returns *v or 0 when v is nil.
func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
