package chart_test

import (
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		fmt      chart.NumberFormat
		decimals int
		want     string
	}{
		{"raw small", 1234.5, chart.NumberFormatRaw, 2, "1,234.50"},
		{"thousands", 1234567, chart.NumberFormatThousands, 1, "1,234.6K"},
		{"millions", 1234567, chart.NumberFormatMillions, 1, "1.2M"},
		{"percent fraction", 0.187, chart.NumberFormatPercent, 1, "18.7%"},
		{"currency thousands", 1234567, chart.NumberFormatCurrencyThousands, 1, "$1,234.6K"},
		{"currency millions", 1234567, chart.NumberFormatCurrencyMillions, 1, "$1.2M"},
		{"negative bracket", -1000, chart.NumberFormatCurrencyThousands, 0, "($1K)"},
		{"negative bracket millions", -2500000, chart.NumberFormatCurrencyMillions, 1, "($2.5M)"},
		{"decimals clamp high", 1234.5, chart.NumberFormatRaw, 99, "1,234.500000"},
		{"decimals clamp low", 1234.5, chart.NumberFormatRaw, -5, "1,234"}, // Go uses banker's rounding
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chart.Format(tc.value, tc.fmt, tc.decimals)
			if got != tc.want {
				t.Errorf("Format(%v, %s, %d): want %q, got %q", tc.value, tc.fmt, tc.decimals, tc.want, got)
			}
		})
	}
}

func TestIsValidNumberFormat(t *testing.T) {
	for _, s := range []string{"raw", "thousands", "millions", "percent", "currency_thousands", "currency_millions"} {
		if !chart.IsValidNumberFormat(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range []string{"", "unknown", "RAW", "currency"} {
		if chart.IsValidNumberFormat(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}
