package validation

import (
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// V12 enforces CMB… <-> product type MB (design §7 V-12; error, coverage):
// coverage lines the coverage step marked INVALID for V-12 (CMB item on a
// non-MB product, or a yarn item on an MB product), and any std row whose
// kind or source disagrees with its item code (yarn never maps to MB).
func V12() Validator {
	return validatorFunc{code: CodeV12, fn: func(in *Input) []Finding {
		var out []Finding
		for _, l := range in.Coverage {
			if isV12Invalid(l) {
				out = append(out, Finding{Code: CodeV12, Severity: SeverityError, Scope: ScopeCoverage, Key: coverageKey(l), Message: l.Reason})
				continue
			}
			if l.Kind != erpintegration.ItemKindForCode(l.ItemCode) {
				out = append(out, Finding{Code: CodeV12, Severity: SeverityError, Scope: ScopeCoverage, Key: coverageKey(l),
					Message: "coverage kind " + string(l.Kind) + " does not match item code " + quote(l.ItemCode)})
			}
		}
		for _, r := range in.Rows {
			want := erpintegration.ItemKindForCode(r.Key.ItemCode)
			if r.Kind != want {
				out = append(out, rowFinding(CodeV12, SeverityError, r.Key,
					"row kind "+string(r.Kind)+" does not match item code "+quote(r.Key.ItemCode)))
				continue
			}
			isMBSource := r.Source == erpintegration.SourceMB
			if r.Source != "" && isMBSource != (want == erpintegration.ItemKindMB) {
				out = append(out, rowFinding(CodeV12, SeverityError, r.Key,
					"source "+string(r.Source)+" does not match item kind "+string(want)))
			}
		}
		return out
	}}
}
