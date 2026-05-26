package chart

import (
	"math"
	"strconv"
	"strings"
)

// NumberFormat is the canonical formatting hint stored in chart_config.number_format.
type NumberFormat string

// All supported number-format keys.
const (
	NumberFormatRaw                NumberFormat = "raw"
	NumberFormatThousands          NumberFormat = "thousands"
	NumberFormatMillions           NumberFormat = "millions"
	NumberFormatPercent            NumberFormat = "percent"
	NumberFormatCurrencyThousands  NumberFormat = "currency_thousands"
	NumberFormatCurrencyMillions   NumberFormat = "currency_millions"
)

// IsValidNumberFormat reports whether the given string is a known number-format key.
func IsValidNumberFormat(s string) bool {
	switch NumberFormat(s) {
	case NumberFormatRaw, NumberFormatThousands, NumberFormatMillions,
		NumberFormatPercent, NumberFormatCurrencyThousands, NumberFormatCurrencyMillions:
		return true
	}
	return false
}

// Format renders a numeric value according to the format key.
//
// Negative values are rendered with accounting-style brackets, e.g. "($1.2K)".
// Decimals is clamped to [0, 6].
func Format(v float64, fmtKey NumberFormat, decimals int) string {
	if decimals < 0 {
		decimals = 0
	}
	if decimals > 6 {
		decimals = 6
	}
	abs := math.Abs(v)
	negative := v < 0

	var body string
	switch fmtKey {
	case NumberFormatRaw:
		body = withCommas(abs, decimals)
	case NumberFormatThousands:
		body = withCommas(abs/1e3, decimals) + "K"
	case NumberFormatMillions:
		body = withCommas(abs/1e6, decimals) + "M"
	case NumberFormatPercent:
		body = withCommas(abs*100, decimals) + "%"
	case NumberFormatCurrencyThousands:
		body = "$" + withCommas(abs/1e3, decimals) + "K"
	case NumberFormatCurrencyMillions:
		body = "$" + withCommas(abs/1e6, decimals) + "M"
	default:
		body = withCommas(abs, decimals)
	}

	if negative {
		return "(" + body + ")"
	}
	return body
}

// withCommas renders a non-negative float with thousands separators.
func withCommas(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	withComma := insertCommas(intPart)
	if len(parts) == 2 {
		return withComma + "." + parts[1]
	}
	return withComma
}

// insertCommas inserts thousand-separator commas into the integer portion of a number.
func insertCommas(intPart string) string {
	n := len(intPart)
	if n <= 3 {
		return intPart
	}
	out := strings.Builder{}
	out.Grow(n + n/3)
	firstGroup := n % 3
	if firstGroup == 0 {
		firstGroup = 3
	}
	out.WriteString(intPart[:firstGroup])
	for i := firstGroup; i < n; i += 3 {
		out.WriteByte(',')
		out.WriteString(intPart[i : i+3])
	}
	return out.String()
}

