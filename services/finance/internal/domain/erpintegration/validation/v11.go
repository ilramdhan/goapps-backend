package validation

import (
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// V11 checks every demanded (item, shade) is linked with coverage OK (design
// §7 V-11; error, batch). A demanded combo without a coverage line, or with a
// non-OK status, is an error. DUP_MAPPING is reported by V-04 and V-12
// INVALID lines by V-12, so they are not repeated here. An empty demand is
// itself an error (nothing to value).
func V11() Validator {
	return validatorFunc{code: CodeV11, fn: func(in *Input) []Finding {
		var out []Finding
		cov := make(map[erpintegration.CoverageKey]erpintegration.CoverageLine, len(in.Coverage))
		for _, l := range in.Coverage {
			cov[l.Key()] = l
		}
		if len(in.Demand) == 0 {
			return []Finding{batchFinding(CodeV11, "batch has no ADJ demand")}
		}
		seen := map[erpintegration.CoverageKey]struct{}{}
		for _, d := range in.Demand {
			k := erpintegration.CoverageKey{ItemCode: d.ItemCode, ShadeCode: d.ShadeCode}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			key := erpintegration.ErpKey{ItemCode: k.ItemCode, ShadeCode: k.ShadeCode}
			l, ok := cov[k]
			switch {
			case !ok:
				out = append(out, Finding{Code: CodeV11, Severity: SeverityError, Scope: ScopeBatch, Key: key,
					Message: "demanded " + k.ItemCode + "/" + k.ShadeCode + " has no coverage line"})
			case l.Status == erpintegration.CoverageOK, l.Status == erpintegration.CoverageDupMapping, isV12Invalid(l):
			default:
				msg := "demanded " + k.ItemCode + "/" + k.ShadeCode + " not covered: " + string(l.Status)
				if l.Reason != "" {
					msg += " (" + l.Reason + ")"
				}
				out = append(out, Finding{Code: CodeV11, Severity: SeverityError, Scope: ScopeBatch, Key: key, Message: msg})
			}
		}
		return out
	}}
}

// isV12Invalid reports whether a coverage line was made INVALID by the V-12
// kind check in the coverage step (reason prefixed "V-12").
func isV12Invalid(l erpintegration.CoverageLine) bool {
	return l.Status == erpintegration.CoverageInvalid && strings.HasPrefix(strings.TrimSpace(l.Reason), string(CodeV12))
}
