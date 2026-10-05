package erpintegration

import "github.com/shopspring/decimal"

// FlexText is the output of FormatFlex: a numeric FLEX_* value rendered as
// Oracle TO_CHAR(ROUND(NVL(x,0),5),'FM990D00000') with
// NLS_NUMERIC_CHARACTERS='.,' (design Part 2 §6.4).
type FlexText string

// String returns the text.
func (f FlexText) String() string { return string(f) }

// FormatFlex is the Go twin of TO_CHAR(ROUND(NVL(x,0),5),'FM990D00000'):
//   - nil is NVL'd to 0;
//   - the value is rounded half away from zero at 5 dp (Round5);
//   - the integer part keeps at least one digit ("0.12000"), the fraction
//     always has exactly 5 digits, FM drops the leading sign blank and a
//     negative value gets a leading "-";
//   - a value that rounds to zero renders without a sign ("0.00000");
//   - |rounded| >= 1000 cannot be rendered by the 990 mask (Oracle emits
//     "#######"), so ErrFlexOverflow is returned instead.
func FormatFlex(x *decimal.Decimal) (FlexText, error) {
	v := decimal.Zero
	if x != nil {
		v = Round5(*x)
	}
	if v.Abs().GreaterThanOrEqual(flexLimit) {
		return "", ErrFlexOverflow
	}
	s := v.Abs().StringFixed(ScaleR5)
	if v.IsNegative() {
		s = "-" + s
	}
	return FlexText(s), nil
}

// FormatFlexNull formats a nullable component; NULL is NVL'd to 0.
func FormatFlexNull(x decimal.NullDecimal) (FlexText, error) {
	if !x.Valid {
		return FormatFlex(nil)
	}
	d := x.Decimal
	return FormatFlex(&d)
}
