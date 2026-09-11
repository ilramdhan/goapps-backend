package rmcost

// Selection Flags V2 — the valuation tier labels and the AUTO cascade that
// resolves one when the configured flag does not name a tier outright.
//
// ⭐ This lives in the DOMAIN package, not in application/rmcost, because two
// callers need it and they sit on opposite sides of an import edge:
//   - application/rmcost, the V2 RM cost engine that computes cost_val, and
//   - infrastructure/postgres, which RE-DERIVES a persisted row's tier for the
//     MB cost-calc-detail export.
//
// application/rmcost imports infrastructure/postgres, so the export could not
// reach the cascade there without an import cycle. Duplicating the cascade
// would let the two copies drift and silently mislabel historical rows, so the
// single implementation was moved down here instead and application/rmcost now
// delegates to it. ⛔ Do not re-inline a second copy in either caller.

// FlagAuto is the cascade-fallback marker for both ValuationFlag and
// MarketingFlag. CL→SL→FL→PR for valuation, SP→PP→FP for marketing.
const FlagAuto = "AUTO"

// FlagNone is the resolved flag when an AUTO cascade finds every candidate
// zero -- CL/SL/FL/PR for valuation, SP/PP/FP for marketing -- meaning no
// source existed for the period. Distinct from the real source labels so
// downstream consumers can tell "resolved to that source" apart from "no
// source at all".
const FlagNone = "NONE"

// ValuationTotals carries the six valuation candidates the cascade chooses
// between. It mirrors the six per-tier rate columns on cst_rm_cost.
type ValuationTotals struct {
	CR float64
	SR float64
	PR float64
	CL float64
	SL float64
	FL float64
}

// MarketingTotals carries the three marketing projection candidates.
type MarketingTotals struct {
	SP float64
	PP float64
	FP float64
}

// LabeledRate pairs a candidate rate with the tier label that produced it.
type LabeledRate struct {
	Value float64
	Label string
}

// SelectValuationWithFlag returns cost_val and the flag actually applied. When
// flag is explicit (CR/SR/PR/CL/SL/FL) the same flag is echoed back. When flag
// is AUTO (or unrecognized), the cascade picks the first non-zero in
// CL→SL→FL→PR and returns the chosen flag — when all four candidates are
// zero, the resolved flag is FlagNone, meaning no price source existed for the
// period (cost_val is 0).
func SelectValuationWithFlag(tot ValuationTotals, flag string) (float64, string) {
	switch flag {
	case "CR":
		return tot.CR, "CR"
	case "SR":
		return tot.SR, "SR"
	case "PR":
		return tot.PR, "PR"
	case "CL":
		return tot.CL, "CL"
	case "SL":
		return tot.SL, "SL"
	case "FL":
		return tot.FL, "FL"
	}
	// AUTO / "" / unknown → cascade.
	v, label := FirstNonZeroWithLabel([]LabeledRate{
		{tot.CL, "CL"}, {tot.SL, "SL"}, {tot.FL, "FL"}, {tot.PR, "PR"},
	})
	if v == 0 {
		// All four candidates are zero: no price source existed for the
		// period. Use the honest FlagNone label instead of echoing back
		// whichever candidate happened to be last in the cascade.
		label = FlagNone
	}
	return v, label
}

// SelectMarketingWithFlag mirrors SelectValuationWithFlag for marketing
// projections: explicit SP/PP/FP echo back, AUTO cascades SP→PP→FP, and an
// all-zero cascade resolves to FlagNone.
func SelectMarketingWithFlag(p MarketingTotals, flag string) (float64, string) {
	switch flag {
	case "SP":
		return p.SP, "SP"
	case "PP":
		return p.PP, "PP"
	case "FP":
		return p.FP, "FP"
	}
	// AUTO / "" / unknown → cascade.
	v, label := FirstNonZeroWithLabel([]LabeledRate{
		{p.SP, "SP"}, {p.PP, "PP"}, {p.FP, "FP"},
	})
	if v == 0 {
		label = FlagNone
	}
	return v, label
}

// FirstNonZeroWithLabel returns the first candidate whose value is strictly
// > 0 along with its label. When every candidate is zero, returns (0, last
// candidate's label); an empty slice returns (0, "").
//
// ⚠ The test is > 0, NOT != 0. A negative rate is treated as "no source" by
// the cascade, exactly as the V2 engine has always treated it.
func FirstNonZeroWithLabel(candidates []LabeledRate) (float64, string) {
	for _, c := range candidates {
		if c.Value > 0 {
			return c.Value, c.Label
		}
	}
	if len(candidates) == 0 {
		return 0, ""
	}
	return 0, candidates[len(candidates)-1].Label
}
