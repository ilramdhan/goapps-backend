package validation

import "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"

// V01 checks that every valued AX (yarn) or MB (grade A) row has std > 0
// (design §7 V-01; error, row). Rows whose status is not OK are reported by
// V-08 / V-03 / V-06 instead.
func V01() Validator {
	return validatorFunc{code: CodeV01, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			if r.Status != erpintegration.DeriveOK {
				continue
			}
			if r.Source != erpintegration.SourceAX && r.Source != erpintegration.SourceMB {
				continue
			}
			if !stdPositive(r) {
				out = append(out, rowFinding(CodeV01, SeverityError, r.Key,
					string(r.Source)+" std cost must be > 0 (is "+stdText(r)+")"))
			}
		}
		return out
	}}
}
