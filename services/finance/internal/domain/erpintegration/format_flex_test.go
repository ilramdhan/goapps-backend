package erpintegration

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dp(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func TestFormatFlex_LegacyFM990D00000(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   *decimal.Decimal
		want string
	}{
		// design §6.4 table
		{"nil is NVL 0", nil, "0.00000"},
		{"tiny positive rounds to zero", dp("0.000004"), "0.00000"},
		{"long tail", dp("12.3456789"), "12.34568"},
		{"just below overflow", dp("999.999994"), "999.99999"},
		{"negative pads fraction", dp("-0.4"), "-0.40000"},
		{"tiny negative rounds to unsigned zero", dp("-0.000004"), "0.00000"},
		// zero forms
		{"zero", dp("0"), "0.00000"},
		{"negative zero", dp("-0.0"), "0.00000"},
		{"zero with scale", dp("0.000000"), "0.00000"},
		// 990 mask keeps one leading zero, FM has no padding
		{"leading zero kept", dp("0.12"), "0.12000"},
		{"integer", dp("7"), "7.00000"},
		{"two digit integer", dp("20"), "20.00000"},
		{"three digit integer", dp("100"), "100.00000"},
		{"max", dp("999.99999"), "999.99999"},
		{"min", dp("-999.99999"), "-999.99999"},
		// half away from zero at 5 dp, both signs
		{"tie up", dp("0.123455"), "0.12346"},
		{"tie up negative", dp("-0.123455"), "-0.12346"},
		{"tie at 5e-6", dp("0.000005"), "0.00001"},
		{"tie at -5e-6", dp("-0.000005"), "-0.00001"},
		{"banker would round to even", dp("0.000025"), "0.00003"},
		{"banker would round to even negative", dp("-0.000025"), "-0.00003"},
		{"tie carries to integer", dp("1.999995"), "2.00000"},
		{"below tie", dp("2.1234549999"), "2.12345"},
		{"already 5 dp", dp("1.23456"), "1.23456"},
		{"negative long tail", dp("-12.3456789"), "-12.34568"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := FormatFlex(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.String())
		})
	}
}

func TestFormatFlex_Overflow_ReturnsErrFlexOverflow(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"999.999995", "1000", "1000.00001", "12345.6", "-999.999995", "-1000", "-1e9"} {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			got, err := FormatFlex(dp(s))
			require.ErrorIs(t, err, ErrFlexOverflow)
			assert.Empty(t, got.String())
		})
	}
}

func TestFormatFlex_DiffersFromBankersRounding(t *testing.T) {
	t.Parallel()
	x := decimal.RequireFromString("0.000025")
	got, err := FormatFlex(&x)
	require.NoError(t, err)
	assert.NotEqual(t, x.RoundBank(ScaleR5).StringFixed(ScaleR5), got.String())
}

func TestFormatFlex_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	x := decimal.RequireFromString("1.2345678")
	_, err := FormatFlex(&x)
	require.NoError(t, err)
	assert.Equal(t, "1.2345678", x.String())
}

func TestFormatFlexNull(t *testing.T) {
	t.Parallel()
	got, err := FormatFlexNull(decimal.NullDecimal{})
	require.NoError(t, err)
	assert.Equal(t, "0.00000", got.String())

	got, err = FormatFlexNull(decimal.NullDecimal{Decimal: decimal.RequireFromString("-1.000005"), Valid: true})
	require.NoError(t, err)
	assert.Equal(t, "-1.00001", got.String())

	_, err = FormatFlexNull(decimal.NullDecimal{Decimal: decimal.NewFromInt(1000), Valid: true})
	require.ErrorIs(t, err, ErrFlexOverflow)
}
