package validation

import "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"

// V06 checks every row's ERP key is push-safe (non-empty, <= 12 printable
// ASCII, no surrounding blanks) and exists in the PG replicas: item in
// cost_erp_item, shade in cost_erp_shade, grade in cost_erp_grade (design §7
// V-06; error, row). Without replica sets it fails closed with one batch
// error. Derive-attached V-06 issues are carried when the recompute finds
// nothing.
func V06() Validator {
	return validatorFunc{code: CodeV06, fn: func(in *Input) []Finding {
		var out []Finding
		if in.Replica == nil && len(in.Rows) > 0 {
			out = append(out, Finding{Code: CodeV06, Severity: SeverityError, Scope: ScopeBatch,
				Message: "ERP master replica sets not provided; existence cannot be verified"})
		}
		for _, r := range in.Rows {
			n := len(out)
			if !r.Key.PushSafe() {
				out = append(out, rowFinding(CodeV06, SeverityError, r.Key,
					"ERP key "+quote(r.Key.String())+" is not push-safe (non-empty, <= 12 ASCII)"))
			}
			if in.Replica != nil {
				out = appendMissing(out, r.Key, "item", r.Key.ItemCode, "cost_erp_item", in.Replica.Items)
				out = appendMissing(out, r.Key, "shade", r.Key.ShadeCode, "cost_erp_shade", in.Replica.Shades)
				out = appendMissing(out, r.Key, "grade", r.Key.GradeCode, "cost_erp_grade", in.Replica.Grades)
			}
			out = carryDerived(out, r, CodeV06, len(out) > n)
		}
		return out
	}}
}

func appendMissing(out []Finding, key erpintegration.ErpKey, what, code, table string, set map[string]struct{}) []Finding {
	if _, ok := set[code]; ok {
		return out
	}
	return append(out, rowFinding(CodeV06, SeverityError, key, what+" "+quote(code)+" not found in "+table))
}

func quote(s string) string { return "\"" + s + "\"" }
