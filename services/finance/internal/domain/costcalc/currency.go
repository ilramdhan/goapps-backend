package costcalc

// Currency labels written to cst_product_cost.cpc_currency_code.
const (
	// CurrencyUSD is the label for ACTUAL results: ACTUAL cost is computed in
	// USD/kg (User decision 2026-09-29 U-3, T-CUR).
	CurrencyUSD = "USD"
	// CurrencyIDR is the historical label, kept for non-ACTUAL calc types so
	// their outputs are unchanged.
	CurrencyIDR = "IDR"
)

// ResultCurrencyFor returns the currency LABEL a freshly computed result is
// written with. It is a label-only decision: the numbers are never converted.
// ACTUAL -> USD (fixes the hard-coded "IDR" mislabel, P0-T10b); every other
// calc type keeps the historical "IDR" label.
func ResultCurrencyFor(calcType CalculationType) string {
	if calcType == CalcTypeActual {
		return CurrencyUSD
	}
	return CurrencyIDR
}
