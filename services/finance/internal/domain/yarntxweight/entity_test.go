package yarntxweight_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

func TestNew_Success(t *testing.T) {
	e, err := yarntxweight.New(3, yarntxweight.GradeAE, yarntxweight.ModeLessBy, 0.5, "desc", "admin")
	require.NoError(t, err)
	assert.Equal(t, int32(3), e.ProductTypeID())
	assert.Equal(t, yarntxweight.GradeAE, e.Grade())
	assert.Equal(t, yarntxweight.ModeLessBy, e.Mode())
	assert.InDelta(t, 0.5, e.Value(), 1e-12)
	assert.False(t, e.IsDeleted())
}

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name string
		pt   int32
		g    yarntxweight.Grade
		m    yarntxweight.Mode
		v    float64
		d    string
		by   string
		want error
	}{
		{"zero product type", 0, yarntxweight.GradeA, yarntxweight.ModeFixed, 1, "", "u", yarntxweight.ErrInvalidProductType},
		{"bad grade", 1, "Z", yarntxweight.ModeFixed, 1, "", "u", yarntxweight.ErrInvalidGrade},
		{"bad mode", 1, yarntxweight.GradeA, "PLUS", 1, "", "u", yarntxweight.ErrInvalidMode},
		{"nan value", 1, yarntxweight.GradeA, yarntxweight.ModeFixed, math.NaN(), "", "u", yarntxweight.ErrInvalidValue},
		{"long description", 1, yarntxweight.GradeA, yarntxweight.ModeFixed, 1, strings.Repeat("x", 201), "u", yarntxweight.ErrDescriptionTooLong},
		{"empty created by", 1, yarntxweight.GradeA, yarntxweight.ModeFixed, 1, "", "", yarntxweight.ErrEmptyCreatedBy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := yarntxweight.New(tc.pt, tc.g, tc.m, tc.v, tc.d, tc.by)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestParseGradeAndMode(t *testing.T) {
	g, err := yarntxweight.ParseGrade(" a9 ")
	require.NoError(t, err)
	assert.Equal(t, yarntxweight.GradeA9, g)
	_, err = yarntxweight.ParseGrade("X")
	assert.ErrorIs(t, err, yarntxweight.ErrInvalidGrade)

	m, err := yarntxweight.ParseMode("multiply")
	require.NoError(t, err)
	assert.Equal(t, yarntxweight.ModeMultiply, m)
	_, err = yarntxweight.ParseMode("")
	assert.ErrorIs(t, err, yarntxweight.ErrInvalidMode)
}

func TestModeApply(t *testing.T) {
	assert.InDelta(t, 92.0, yarntxweight.ModeLessBy.Apply(100, 8), 1e-12)
	assert.InDelta(t, 65.0, yarntxweight.ModeMultiply.Apply(100, 0.65), 1e-9)
	assert.InDelta(t, 2.5, yarntxweight.ModeFixed.Apply(100, 2.5), 1e-12)
}

func TestUpdate(t *testing.T) {
	e, err := yarntxweight.New(1, yarntxweight.GradeB, yarntxweight.ModeFixed, 1, "", "admin")
	require.NoError(t, err)

	mode := yarntxweight.ModeMultiply
	val := 0.7
	desc := "changed"
	require.NoError(t, e.Update(yarntxweight.UpdateInput{Mode: &mode, Value: &val, Description: &desc}, "editor"))
	assert.Equal(t, yarntxweight.ModeMultiply, e.Mode())
	assert.InDelta(t, 0.7, e.Value(), 1e-12)
	assert.Equal(t, "changed", e.Description())
	require.NotNil(t, e.UpdatedBy())
	assert.Equal(t, "editor", *e.UpdatedBy())

	bad := yarntxweight.Mode("NOPE")
	assert.ErrorIs(t, e.Update(yarntxweight.UpdateInput{Mode: &bad}, "editor"), yarntxweight.ErrInvalidMode)
	assert.Equal(t, yarntxweight.ModeMultiply, e.Mode(), "failed update must not mutate")
}

func TestSoftDelete(t *testing.T) {
	e, err := yarntxweight.New(1, yarntxweight.GradeC, yarntxweight.ModeFixed, 0.7, "", "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.True(t, e.IsDeleted())
	assert.ErrorIs(t, e.SoftDelete("admin"), yarntxweight.ErrAlreadyDeleted)
	v := 1.0
	assert.ErrorIs(t, e.Update(yarntxweight.UpdateInput{Value: &v}, "admin"), yarntxweight.ErrAlreadyDeleted)
}
