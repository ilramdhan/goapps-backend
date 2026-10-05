package validation

import (
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// DeltaThreshold is the V-05 relative change bound (20%).
var DeltaThreshold = decimal.RequireFromString("0.20")

// V05 compares each valued row's std with the previous active batch's std
// for the same key (design §7 V-05, Q11). |Δ| / |prev| > 20% is a warning
// that requires an ack; exactly 20% passes. A key without a previous std is
// first-seen: info only. A zero previous std with a non-zero current std is
// a warning (unbounded change).
func V05() Validator {
	return validatorFunc{code: CodeV05, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			if r.Status != erpintegration.DeriveOK || !r.StdCost.Valid {
				continue
			}
			cur := r.StdCost.Decimal
			prev, ok := in.PrevStd[r.Key]
			if !ok {
				out = append(out, rowFinding(CodeV05, SeverityInfo, r.Key, "first-seen combo: no previous active std"))
				continue
			}
			if prev.IsZero() {
				if !cur.IsZero() {
					out = append(out, rowFinding(CodeV05, SeverityWarning, r.Key,
						"std "+cur.String()+" vs previous 0: change is unbounded"))
				}
				continue
			}
			rel := cur.Sub(prev).Abs().Div(prev.Abs())
			if rel.GreaterThan(DeltaThreshold) {
				pct := rel.Mul(hundred).Round(2)
				out = append(out, rowFinding(CodeV05, SeverityWarning, r.Key,
					"std "+cur.String()+" vs previous "+prev.String()+": change "+pct.StringFixed(2)+"% > 20%"))
			}
		}
		return out
	}}
}
