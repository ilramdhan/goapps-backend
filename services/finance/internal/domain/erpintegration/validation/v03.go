package validation

import (
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// FlexBound is the largest magnitude a component may have: FM990D00000
// renders [-999.99999, 999.99999] (design §7 V-03).
var FlexBound = decimal.RequireFromString("999.99999")

// V03 checks every set numeric component is within ±FlexBound and that
// chp_con_kg > 0 (design §7 V-03; error, row). chp_con_kg is required on OK
// rows; on non-OK rows it is checked only when set. Derive-attached V-03
// issues are carried when the recompute finds nothing.
func V03() Validator {
	return validatorFunc{code: CodeV03, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			n := len(out)
			for _, c := range r.NumericComponents() {
				if c.Value.Valid && c.Value.Decimal.Abs().GreaterThan(FlexBound) {
					out = append(out, rowFinding(CodeV03, SeverityError, r.Key,
						c.Name+" "+c.Value.Decimal.String()+" is outside [-999.99999, 999.99999]"))
				}
			}
			needKg := r.Status == erpintegration.DeriveOK || r.ChpConKg.Valid
			if needKg && (!r.ChpConKg.Valid || !r.ChpConKg.Decimal.IsPositive()) {
				out = append(out, rowFinding(CodeV03, SeverityError, r.Key, "chp_con_kg must be > 0"))
			}
			out = carryDerived(out, r, CodeV03, len(out) > n)
		}
		return out
	}}
}
