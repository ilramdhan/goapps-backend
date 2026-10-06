package erpintegration

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRound5_HalfAwayFromZero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		// design §6.3 pinned cases
		{"0.123455", "0.12346"},
		{"-0.123455", "-0.12346"},
		{"1.000005", "1.00001"},
		{"0.0000025", "0"},
		{"-0.0000025", "0"},
		{"0.000005", "0.00001"},
		{"-0.000005", "-0.00001"},
		// ties at the 6th place, both signs
		{"0.000015", "0.00002"},
		{"0.000025", "0.00003"}, // banker's would give 0.00002
		{"-0.000025", "-0.00003"},
		{"2.345675", "2.34568"},
		{"-2.345675", "-2.34568"},
		{"999.999995", "1000"},
		{"-999.999995", "-1000"},
		{"999.999994", "999.99999"},
		// below / above the tie
		{"0.1234549999", "0.12345"},
		{"0.1234550001", "0.12346"},
		{"-0.1234549999", "-0.12345"},
		{"-0.1234550001", "-0.12346"},
		// already at or below scale
		{"12.34567", "12.34567"},
		{"12.3", "12.3"},
		{"0", "0"},
		{"-0", "0"},
		{"100", "100"},
		// long tails
		{"12.3456789", "12.34568"},
		{"-12.3456789", "-12.34568"},
		{"0.0000049999", "0"},
		{"0.00000500000001", "0.00001"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := Round5(decimal.RequireFromString(tc.in))
			want := decimal.RequireFromString(tc.want)
			assert.Truef(t, got.Equal(want), "Round5(%s) = %s, want %s", tc.in, got, tc.want)
		})
	}
}

func TestRound5_DiffersFromBankers(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"0.000025", "-0.000025", "1.234565", "0.000045"} {
		d := decimal.RequireFromString(s)
		assert.Falsef(t, Round5(d).Equal(d.RoundBank(ScaleR5)),
			"Round5(%s) must differ from RoundBank (half-away-from-zero required)", s)
	}
}

func TestParseDecimal(t *testing.T) {
	t.Parallel()
	d, err := ParseDecimal("  12.500000 ")
	require.NoError(t, err)
	assert.True(t, d.Equal(decimal.RequireFromString("12.5")))

	_, err = ParseDecimal("")
	assert.True(t, errors.Is(err, ErrInvalidDecimal))
	_, err = ParseDecimal("abc")
	assert.True(t, errors.Is(err, ErrInvalidDecimal))
}

func TestParseNullDecimal(t *testing.T) {
	t.Parallel()
	nd, err := ParseNullDecimal(nil)
	require.NoError(t, err)
	assert.False(t, nd.Valid)

	blank := " "
	nd, err = ParseNullDecimal(&blank)
	require.NoError(t, err)
	assert.False(t, nd.Valid)

	v := "-0.40000"
	nd, err = ParseNullDecimal(&v)
	require.NoError(t, err)
	assert.True(t, nd.Valid)
	assert.True(t, nd.Decimal.Equal(decimal.RequireFromString("-0.4")))

	bad := "x1"
	_, err = ParseNullDecimal(&bad)
	assert.True(t, errors.Is(err, ErrInvalidDecimal))
}

func TestScanNullDecimal(t *testing.T) {
	t.Parallel()
	nd, err := ScanNullDecimal(nil)
	require.NoError(t, err)
	assert.False(t, nd.Valid)

	nd, err = ScanNullDecimal([]byte("1.234565"))
	require.NoError(t, err)
	assert.True(t, nd.Valid)
	assert.True(t, Round5(nd.Decimal).Equal(decimal.RequireFromString("1.23457")))

	nd, err = ScanNullDecimal("7")
	require.NoError(t, err)
	assert.True(t, nd.Decimal.Equal(decimal.NewFromInt(7)))

	_, err = ScanNullDecimal("not-a-number")
	assert.True(t, errors.Is(err, ErrInvalidDecimal))
}
