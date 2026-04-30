// Package rmgroup — V2 flag types for the RM Cost V2 engine.
// V2 uses two new orthogonal flag domains rather than the unified V1 Flag enum.
package rmgroup

import "strings"

// ValuationFlag selects which computed rate feeds cost_val. AUTO triggers a
// CL→SL→FL fallback. Distinct from the V1 Flag enum.
type ValuationFlag string

// ValuationFlag constants — MUST match the chk_rm_group_valuation_flag_v2
// CHECK constraint and the cst_rm_cost.valuation_flag_v2 column.
const (
	// ValuationFlagAuto picks the first non-zero rate from CL, SL, FL in order.
	ValuationFlagAuto ValuationFlag = "AUTO"
	// ValuationFlagCR forces Consumption Rate (group total cons_val/cons_qty).
	ValuationFlagCR ValuationFlag = "CR"
	// ValuationFlagSR forces Stock Rate.
	ValuationFlagSR ValuationFlag = "SR"
	// ValuationFlagPR forces PO Rate.
	ValuationFlagPR ValuationFlag = "PR"
	// ValuationFlagCL forces Consumption Landed Cost.
	ValuationFlagCL ValuationFlag = "CL"
	// ValuationFlagSL forces Stock Landed Cost.
	ValuationFlagSL ValuationFlag = "SL"
	// ValuationFlagFL forces Fix Landed Cost (max of per-detail FL).
	ValuationFlagFL ValuationFlag = "FL"
)

// ParseValuationFlag validates and returns a ValuationFlag.
// Empty input maps to AUTO. Unknown raises ErrInvalidFlag.
func ParseValuationFlag(raw string) (ValuationFlag, error) {
	cleaned := strings.ToUpper(strings.TrimSpace(raw))
	if cleaned == "" {
		return ValuationFlagAuto, nil
	}
	switch ValuationFlag(cleaned) {
	case ValuationFlagAuto,
		ValuationFlagCR,
		ValuationFlagSR,
		ValuationFlagPR,
		ValuationFlagCL,
		ValuationFlagSL,
		ValuationFlagFL:
		return ValuationFlag(cleaned), nil
	default:
		return "", ErrInvalidFlag
	}
}

// IsValid reports whether the flag is one of the recognized values.
func (f ValuationFlag) IsValid() bool {
	switch f {
	case ValuationFlagAuto,
		ValuationFlagCR,
		ValuationFlagSR,
		ValuationFlagPR,
		ValuationFlagCL,
		ValuationFlagSL,
		ValuationFlagFL:
		return true
	default:
		return false
	}
}

// IsAuto reports whether the flag is AUTO (cascade fallback).
func (f ValuationFlag) IsAuto() bool { return f == "" || f == ValuationFlagAuto }

// String returns the canonical string form.
func (f ValuationFlag) String() string { return string(f) }

// MarketingFlag selects which projection feeds cost_mark. AUTO triggers a
// SP→PP→FP fallback.
type MarketingFlag string

// MarketingFlag constants — MUST match the chk_rm_group_marketing_flag_v2
// CHECK constraint.
const (
	// MarketingFlagAuto picks the first non-zero from SP, PP, FP in order.
	MarketingFlagAuto MarketingFlag = "AUTO"
	// MarketingFlagSP forces Projection Stock Landed Cost.
	MarketingFlagSP MarketingFlag = "SP"
	// MarketingFlagPP forces Projection PO Landed Cost.
	MarketingFlagPP MarketingFlag = "PP"
	// MarketingFlagFP forces Projection Fix Value Landed Cost.
	MarketingFlagFP MarketingFlag = "FP"
)

// ParseMarketingFlag validates and returns a MarketingFlag. Empty maps to AUTO.
func ParseMarketingFlag(raw string) (MarketingFlag, error) {
	cleaned := strings.ToUpper(strings.TrimSpace(raw))
	if cleaned == "" {
		return MarketingFlagAuto, nil
	}
	switch MarketingFlag(cleaned) {
	case MarketingFlagAuto, MarketingFlagSP, MarketingFlagPP, MarketingFlagFP:
		return MarketingFlag(cleaned), nil
	default:
		return "", ErrInvalidFlag
	}
}

// IsValid reports whether the flag is recognized.
func (f MarketingFlag) IsValid() bool {
	switch f {
	case MarketingFlagAuto, MarketingFlagSP, MarketingFlagPP, MarketingFlagFP:
		return true
	default:
		return false
	}
}

// IsAuto reports whether the flag is AUTO.
func (f MarketingFlag) IsAuto() bool { return f == "" || f == MarketingFlagAuto }

// String returns the canonical string form.
func (f MarketingFlag) String() string { return string(f) }

// MarketingInputs is the per-head bag of marketing-projection inputs.
// All fields nullable — NULL means "use 0" in formulas, not "missing required value".
type MarketingInputs struct {
	FreightRate     *float64
	AntiDumpingPct  *float64 // whole percent (5 = 5%) at storage; engine divides by 100
	DefaultValue    *float64
	ValuationFlag   ValuationFlag
	MarketingFlag   MarketingFlag
}

// ValuationInputs is the per-detail bag of valuation-formula inputs.
type ValuationInputs struct {
	FreightRate    *float64
	AntiDumpingPct *float64 // decimal at storage (0.10 = 10%)
	DutyPct        *float64 // decimal at storage
	TransportRate  *float64
	DefaultValue   *float64
}
