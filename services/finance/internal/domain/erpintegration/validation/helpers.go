package validation

import (
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// derivedIssues returns the issues with the given code that Derive attached
// to a row (carried into validation so nothing Derive found is lost).
func derivedIssues(r erpintegration.StdRow, code erpintegration.IssueCode) []erpintegration.Issue {
	var out []erpintegration.Issue
	for _, is := range r.Issues {
		if is.Code == code {
			out = append(out, is)
		}
	}
	return out
}

// carryDerived appends a finding for every derive-attached issue of code
// when the recompute produced none for the row (no duplicates).
func carryDerived(out []Finding, r erpintegration.StdRow, code erpintegration.IssueCode, recomputed bool) []Finding {
	if recomputed {
		return out
	}
	for _, is := range derivedIssues(r, code) {
		out = append(out, rowFinding(code, is.Severity, r.Key, is.Message))
	}
	return out
}

// stdPositive reports whether the row's std cost is set and > 0.
func stdPositive(r erpintegration.StdRow) bool {
	return r.StdCost.Valid && r.StdCost.Decimal.IsPositive()
}

func stdText(r erpintegration.StdRow) string {
	if !r.StdCost.Valid {
		return "NULL"
	}
	return r.StdCost.Decimal.String()
}

var hundred = decimal.NewFromInt(100)
