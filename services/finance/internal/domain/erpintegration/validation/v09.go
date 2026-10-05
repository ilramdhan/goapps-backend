package validation

import (
	"strconv"
	"strings"
)

// CurrencyUSD is the only label accepted once 000557 relabelled the period.
const CurrencyUSD = "USD"

// V09 checks the currency of the source cst_product_cost row of every row
// with an AX cost id (design §7 V-09, §7.3; error, row): strict USD once the
// relabel is applied for the period (or when no interim policy is given),
// otherwise the interim CurrencyPolicy (P0-T10a). A missing source cost fails
// closed.
func V09() Validator {
	return validatorFunc{code: CodeV09, fn: func(in *Input) []Finding {
		var out []Finding
		for _, r := range in.Rows {
			if r.AxCostSysID == nil {
				continue
			}
			id := *r.AxCostSysID
			c, ok := in.SourceCosts[id]
			if !ok {
				out = append(out, rowFinding(CodeV09, SeverityError, r.Key,
					"source cost "+strconv.FormatInt(id, 10)+" not provided; currency cannot be verified"))
				continue
			}
			if currencyOK(in, r.Key.ItemCode, c) {
				continue
			}
			out = append(out, rowFinding(CodeV09, SeverityError, r.Key,
				"source cost "+strconv.FormatInt(id, 10)+" currency "+quote(c.Currency)+" value "+c.CostPerUnit.String()+
					" fails the USD rule for "+in.Period))
		}
		return out
	}}
}

func currencyOK(in *Input, itemCode string, c SourceCost) bool {
	if in.CurrencyRelabelApplied || in.CurrencyPolicy == nil {
		return strings.EqualFold(strings.TrimSpace(c.Currency), CurrencyUSD)
	}
	return in.CurrencyPolicy(in.Period, itemCode, c)
}
