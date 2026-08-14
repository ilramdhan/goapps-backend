package mbhead_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestStringValueObjects exercises each bounded string value object at its empty, valid,
// boundary and over-length points (spec section 2.1).
func TestStringValueObjects(t *testing.T) {
	tests := []struct {
		name       string
		maxLen     int
		build      func(string) (string, error)
		errEmpty   error
		errTooLong error
	}{
		{
			name: "MgtName", maxLen: 100,
			build:    func(s string) (string, error) { v, err := mbhead.NewMgtName(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyMgtName, errTooLong: mbhead.ErrMgtNameTooLong,
		},
		{
			name: "DevCode", maxLen: 50,
			build:    func(s string) (string, error) { v, err := mbhead.NewDevCode(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyDevCode, errTooLong: mbhead.ErrDevCodeTooLong,
		},
		{
			name: "VsNumber", maxLen: 50,
			build:    func(s string) (string, error) { v, err := mbhead.NewVsNumber(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyVsNumber, errTooLong: mbhead.ErrVsNumberTooLong,
		},
		{
			name: "NoOfProcess", maxLen: 10,
			build:    func(s string) (string, error) { v, err := mbhead.NewNoOfProcess(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyNoOfProcess, errTooLong: mbhead.ErrNoOfProcessTooLong,
		},
		{
			name: "ShadeCode", maxLen: 20,
			build:    func(s string) (string, error) { v, err := mbhead.NewShadeCode(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyShadeCode, errTooLong: mbhead.ErrShadeCodeTooLong,
		},
		{
			name: "ShadeName", maxLen: 100,
			build:    func(s string) (string, error) { v, err := mbhead.NewShadeName(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyShadeName, errTooLong: mbhead.ErrShadeNameTooLong,
		},
		{
			name: "CrossSection", maxLen: 20,
			build:    func(s string) (string, error) { v, err := mbhead.NewCrossSection(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyCrossSection, errTooLong: mbhead.ErrCrossSectionTooLong,
		},
		{
			name: "FinalProduct", maxLen: 200,
			build:    func(s string) (string, error) { v, err := mbhead.NewFinalProduct(s); return v.String(), err },
			errEmpty: mbhead.ErrEmptyFinalProduct, errTooLong: mbhead.ErrFinalProductTooLong,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build("")
			assert.ErrorIs(t, err, tt.errEmpty)

			_, err = tt.build("   ")
			assert.ErrorIs(t, err, tt.errEmpty, "whitespace-only must trim to empty")

			atMax := strings.Repeat("a", tt.maxLen)
			got, err := tt.build(atMax)
			require.NoError(t, err)
			assert.Equal(t, atMax, got)

			_, err = tt.build(strings.Repeat("a", tt.maxLen+1))
			assert.ErrorIs(t, err, tt.errTooLong)

			got, err = tt.build("  trimmed  ")
			require.NoError(t, err)
			assert.Equal(t, "trimmed", got)
		})
	}
}

func TestNewDenier(t *testing.T) {
	v, err := mbhead.NewDenier(150.5)
	require.NoError(t, err)
	assert.InDelta(t, 150.5, v.Float64(), 0.001)

	_, err = mbhead.NewDenier(0)
	assert.ErrorIs(t, err, mbhead.ErrInvalidDenier)
	_, err = mbhead.NewDenier(-0.1)
	assert.ErrorIs(t, err, mbhead.ErrInvalidDenier)
}

func TestNewFilament(t *testing.T) {
	v, err := mbhead.NewFilament(48)
	require.NoError(t, err)
	assert.Equal(t, 48, v.Int())

	_, err = mbhead.NewFilament(0)
	assert.ErrorIs(t, err, mbhead.ErrInvalidFilament)
	_, err = mbhead.NewFilament(-1)
	assert.ErrorIs(t, err, mbhead.ErrInvalidFilament)
}

func TestNewLdrPercent(t *testing.T) {
	for _, v := range []float64{0, 50.5, 100} {
		got, err := mbhead.NewLdrPercent(v)
		require.NoError(t, err)
		assert.InDelta(t, v, got.Float64(), 0.001)
	}
	_, err := mbhead.NewLdrPercent(-0.1)
	assert.ErrorIs(t, err, mbhead.ErrInvalidLdrPercent)
	_, err = mbhead.NewLdrPercent(100.1)
	assert.ErrorIs(t, err, mbhead.ErrInvalidLdrPercent)
}

// TestNewNoOfProcess_DoesNotHardcodeOptions guards spec section 2.3: the S/D/T set lives in
// mst_mb_param_option and is checked by the application layer, so the domain must accept any
// well-formed code.
func TestNewNoOfProcess_DoesNotHardcodeOptions(t *testing.T) {
	for _, code := range []string{"S", "D", "T", "Q"} {
		v, err := mbhead.NewNoOfProcess(code)
		require.NoError(t, err, code)
		assert.Equal(t, code, v.String())
	}
}
