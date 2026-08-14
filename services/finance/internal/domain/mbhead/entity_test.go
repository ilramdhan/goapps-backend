package mbhead_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// validInput returns a NewInput with every required field (spec section 2.1) populated.
func validInput() mbhead.NewInput {
	return mbhead.NewInput{
		MBCosting:    "MBH-2024-001",
		MgtName:      "MB Head A",
		DevCode:      "DEV-001",
		VsNumber:     "VS-001",
		NoOfProcess:  "S",
		ShadeCode:    "SH-01",
		ShadeName:    "Shade One",
		CrossSection: "ROUND",
		FinalProduct: "MELANGE YARN 150D/48F",
		Denier:       150,
		Filament:     48,
		LdrPrsn:      12.5,
		CreatedBy:    "admin",
	}
}

func TestNew_Success(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)
	assert.Equal(t, "MBH-2024-001", e.MBCosting())
	require.NotNil(t, e.MgtName())
	assert.Equal(t, "MB Head A", *e.MgtName())
	assert.Equal(t, "DEV-001", e.DevCode())
	assert.Equal(t, "VS-001", e.VsNumber())
	assert.Equal(t, "S", e.NoOfProcess())
	assert.Equal(t, "SH-01", e.ShadeCode())
	assert.Equal(t, "Shade One", e.ShadeName())
	assert.Equal(t, "ROUND", e.CrossSection())
	require.NotNil(t, e.MBHFinalProduct())
	assert.Equal(t, "MELANGE YARN 150D/48F", *e.MBHFinalProduct())
	require.NotNil(t, e.Denier())
	assert.InDelta(t, 150.0, *e.Denier(), 0.001)
	require.NotNil(t, e.Filament())
	assert.Equal(t, 48, *e.Filament())
	require.NotNil(t, e.MBHLdrPrsn())
	assert.InDelta(t, 12.5, *e.MBHLdrPrsn(), 0.001)
	assert.True(t, e.IsActive())
	assert.Empty(t, e.Shades())
}

// TestNew_RequiredFieldRejections covers every required field from spec section 2.1, in both
// the empty and the over-length direction where a length bound exists.
func TestNew_RequiredFieldRejections(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*mbhead.NewInput)
		wantErr error
	}{
		{"empty mb_costing", func(in *mbhead.NewInput) { in.MBCosting = "" }, mbhead.ErrEmptyMBCosting},
		{"mb_costing too long", func(in *mbhead.NewInput) { in.MBCosting = strings.Repeat("a", 101) }, mbhead.ErrMBCostingTooLong},

		{"empty mgt_name", func(in *mbhead.NewInput) { in.MgtName = "" }, mbhead.ErrEmptyMgtName},
		{"mgt_name too long", func(in *mbhead.NewInput) { in.MgtName = strings.Repeat("a", 101) }, mbhead.ErrMgtNameTooLong},

		{"empty dev_code", func(in *mbhead.NewInput) { in.DevCode = "" }, mbhead.ErrEmptyDevCode},
		{"dev_code too long", func(in *mbhead.NewInput) { in.DevCode = strings.Repeat("a", 51) }, mbhead.ErrDevCodeTooLong},

		{"empty vs_number", func(in *mbhead.NewInput) { in.VsNumber = "" }, mbhead.ErrEmptyVsNumber},
		{"vs_number too long", func(in *mbhead.NewInput) { in.VsNumber = strings.Repeat("a", 51) }, mbhead.ErrVsNumberTooLong},

		{"empty no_of_process", func(in *mbhead.NewInput) { in.NoOfProcess = "" }, mbhead.ErrEmptyNoOfProcess},
		{"no_of_process too long", func(in *mbhead.NewInput) { in.NoOfProcess = strings.Repeat("a", 11) }, mbhead.ErrNoOfProcessTooLong},

		{"empty shade_code", func(in *mbhead.NewInput) { in.ShadeCode = "" }, mbhead.ErrEmptyShadeCode},
		{"shade_code too long", func(in *mbhead.NewInput) { in.ShadeCode = strings.Repeat("a", 21) }, mbhead.ErrShadeCodeTooLong},

		{"empty shade_name", func(in *mbhead.NewInput) { in.ShadeName = "" }, mbhead.ErrEmptyShadeName},
		{"shade_name too long", func(in *mbhead.NewInput) { in.ShadeName = strings.Repeat("a", 101) }, mbhead.ErrShadeNameTooLong},

		{"empty cross_section", func(in *mbhead.NewInput) { in.CrossSection = "" }, mbhead.ErrEmptyCrossSection},
		{"cross_section too long", func(in *mbhead.NewInput) { in.CrossSection = strings.Repeat("a", 21) }, mbhead.ErrCrossSectionTooLong},

		{"empty final_product", func(in *mbhead.NewInput) { in.FinalProduct = "" }, mbhead.ErrEmptyFinalProduct},
		{"final_product too long", func(in *mbhead.NewInput) { in.FinalProduct = strings.Repeat("a", 201) }, mbhead.ErrFinalProductTooLong},

		{"zero denier", func(in *mbhead.NewInput) { in.Denier = 0 }, mbhead.ErrInvalidDenier},
		{"negative denier", func(in *mbhead.NewInput) { in.Denier = -1 }, mbhead.ErrInvalidDenier},

		{"zero filament", func(in *mbhead.NewInput) { in.Filament = 0 }, mbhead.ErrInvalidFilament},
		{"negative filament", func(in *mbhead.NewInput) { in.Filament = -1 }, mbhead.ErrInvalidFilament},

		{"ldr below range", func(in *mbhead.NewInput) { in.LdrPrsn = -0.1 }, mbhead.ErrInvalidLdrPercent},
		{"ldr above range", func(in *mbhead.NewInput) { in.LdrPrsn = 100.1 }, mbhead.ErrInvalidLdrPercent},

		{"empty created_by", func(in *mbhead.NewInput) { in.CreatedBy = "" }, mbhead.ErrEmptyCreatedBy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			_, err := mbhead.New(in)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNew_LdrBoundsInclusive(t *testing.T) {
	for _, v := range []float64{0, 100} {
		in := validInput()
		in.LdrPrsn = v
		_, err := mbhead.New(in)
		assert.NoError(t, err)
	}
}

func TestNew_TrimsWhitespace(t *testing.T) {
	in := validInput()
	in.DevCode = "  DEV-002  "
	e, err := mbhead.New(in)
	require.NoError(t, err)
	assert.Equal(t, "DEV-002", e.DevCode())
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), mbhead.ErrAlreadyDeleted)
}

func TestUpdate_Success(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)

	vs := "VS-999"
	nop := "T"
	name := "Renamed"
	den := 300.0
	fil := 96
	require.NoError(t, e.Update(mbhead.UpdateInput{
		VsNumber:    &vs,
		NoOfProcess: &nop,
		MgtName:     &name,
		Denier:      &den,
		Filament:    &fil,
	}, "editor"))

	assert.Equal(t, "VS-999", e.VsNumber())
	assert.Equal(t, "T", e.NoOfProcess())
	require.NotNil(t, e.MgtName())
	assert.Equal(t, "Renamed", *e.MgtName())
	require.NotNil(t, e.Denier())
	assert.InDelta(t, 300.0, *e.Denier(), 0.001)
	require.NotNil(t, e.Filament())
	assert.Equal(t, 96, *e.Filament())
	assert.NotNil(t, e.UpdatedAt())
}

// TestUpdate_RequiredFieldRejections verifies the update path validates supplied required
// fields rather than accepting a blank value.
func TestUpdate_RequiredFieldRejections(t *testing.T) {
	blank := ""
	long101 := strings.Repeat("a", 101)
	zero := 0.0
	zeroInt := 0
	overLdr := 100.1

	tests := []struct {
		name    string
		in      mbhead.UpdateInput
		wantErr error
	}{
		{"blank mb_costing", mbhead.UpdateInput{MBCosting: &blank}, mbhead.ErrEmptyMBCosting},
		{"blank mgt_name", mbhead.UpdateInput{MgtName: &blank}, mbhead.ErrEmptyMgtName},
		{"long mgt_name", mbhead.UpdateInput{MgtName: &long101}, mbhead.ErrMgtNameTooLong},
		{"blank dev_code", mbhead.UpdateInput{DevCode: &blank}, mbhead.ErrEmptyDevCode},
		{"blank vs_number", mbhead.UpdateInput{VsNumber: &blank}, mbhead.ErrEmptyVsNumber},
		{"blank no_of_process", mbhead.UpdateInput{NoOfProcess: &blank}, mbhead.ErrEmptyNoOfProcess},
		{"blank shade_code", mbhead.UpdateInput{ShadeCode: &blank}, mbhead.ErrEmptyShadeCode},
		{"blank shade_name", mbhead.UpdateInput{ShadeName: &blank}, mbhead.ErrEmptyShadeName},
		{"blank cross_section", mbhead.UpdateInput{CrossSection: &blank}, mbhead.ErrEmptyCrossSection},
		{"blank final_product", mbhead.UpdateInput{MBHFinalProduct: &blank}, mbhead.ErrEmptyFinalProduct},
		{"zero denier", mbhead.UpdateInput{Denier: &zero}, mbhead.ErrInvalidDenier},
		{"zero filament", mbhead.UpdateInput{Filament: &zeroInt}, mbhead.ErrInvalidFilament},
		{"ldr out of range", mbhead.UpdateInput{MBHLdrPrsn: &overLdr}, mbhead.ErrInvalidLdrPercent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := mbhead.New(validInput())
			require.NoError(t, err)
			assert.ErrorIs(t, e.Update(tt.in, "editor"), tt.wantErr)
		})
	}
}

func TestUpdate_AlreadyDeleted(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.Update(mbhead.UpdateInput{}, "editor"), mbhead.ErrAlreadyDeleted)
}
