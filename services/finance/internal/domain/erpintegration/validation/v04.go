package validation

import (
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// V04 reports every ambiguous ERP link: a coverage combo with more than one
// active AX product for (cpm_erp_item_code, cpm_shade_code) (design §7 V-04,
// D-LINK; error, coverage). The combo is blocked, never guessed.
func V04() Validator {
	return validatorFunc{code: CodeV04, fn: func(in *Input) []Finding {
		var out []Finding
		for _, l := range in.Coverage {
			if l.Status != erpintegration.CoverageDupMapping {
				continue
			}
			msg := fmt.Sprintf("ambiguous mapping: more than one active AX product for %s/%s", l.ItemCode, l.ShadeCode)
			if l.Reason != "" {
				msg = l.Reason
			}
			out = append(out, Finding{Code: CodeV04, Severity: SeverityError, Scope: ScopeCoverage, Key: coverageKey(l), Message: msg})
		}
		return out
	}}
}
