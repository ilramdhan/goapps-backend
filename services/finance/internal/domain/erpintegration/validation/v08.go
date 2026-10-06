package validation

import "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"

// V08 reports every row whose derivation status is not OK (missing AX cost,
// rule, grade group, FG type or selling price; error), and the V-08w prefix
// fallback warning (C-18: prefix not POY/PTY/ITY valued as PTY; warning,
// ack required) on derived rows (design §7 V-08, row scope).
func V08() Validator {
	return validatorFunc{code: CodeV08, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			out = append(out, v08Row(r)...)
			out = append(out, v08wRow(r)...)
		}
		return out
	}}
}

func v08Row(r erpintegration.StdRow) []Finding {
	if r.Status == erpintegration.DeriveOK || r.Status == erpintegration.DeriveInvalid {
		// INVALID is a V-03 / V-06 failure, reported by those validators.
		return nil
	}
	msg := string(r.Status) + ": derivation input missing"
	for _, is := range derivedIssues(r, CodeV08) {
		msg = is.Message
		break
	}
	return []Finding{rowFinding(CodeV08, SeverityError, r.Key, msg)}
}

func v08wRow(r erpintegration.StdRow) []Finding {
	if r.Source != erpintegration.SourceDerived {
		return nil
	}
	if _, fallback := erpintegration.ProdTypeForItem(r.Key.ItemCode); fallback {
		return []Finding{rowFinding(CodeV08w, SeverityWarning, r.Key,
			"item prefix of "+quote(r.Key.ItemCode)+" is not POY/PTY/ITY; prod type falls back to PTY")}
	}
	return nil
}
