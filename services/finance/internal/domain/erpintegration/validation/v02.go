package validation

import "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"

// V02 checks that every valued derived grade row has std > 0 (design §7
// V-02; error, row). A selling-price basis with a value loss above the price
// is the typical failure.
func V02() Validator {
	return validatorFunc{code: CodeV02, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			if r.Status != erpintegration.DeriveOK || r.Source != erpintegration.SourceDerived {
				continue
			}
			if !stdPositive(r) {
				out = append(out, rowFinding(CodeV02, SeverityError, r.Key,
					"derived std cost must be > 0 (is "+stdText(r)+", basis "+string(r.Basis)+")"))
			}
		}
		return out
	}}
}
