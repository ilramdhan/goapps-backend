package yarntxweightgroup_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

func ttyRules() []yarntxweightgroup.Rule {
	return []yarntxweightgroup.Rule{
		{Grade: yarntxweight.GradeC, Mode: yarntxweight.ModeFixed, Value: 0.7},
		{Grade: yarntxweight.GradeAE, Mode: yarntxweight.ModeLessBy, Value: 0.5},
		{Grade: "a9", Mode: "multiply", Value: 0.7},
	}
}

func validInput() yarntxweightgroup.Input {
	return yarntxweightgroup.Input{
		Code: " tty ", Name: " Twisted Yarn ", Description: "shared",
		ProductTypeIDs: []int32{3, 7, 9}, Rules: ttyRules(),
	}
}

func TestNew_Success(t *testing.T) {
	e, err := yarntxweightgroup.New(validInput(), "admin")
	require.NoError(t, err)
	assert.Equal(t, "TTY", e.Code(), "code upper-cased and trimmed")
	assert.Equal(t, "Twisted Yarn", e.Name())
	assert.Equal(t, []int32{3, 7, 9}, e.ProductTypeIDs())
	rules := e.Rules()
	require.Len(t, rules, 3)
	assert.Equal(t, yarntxweight.GradeAE, rules[0].Grade, "rules sorted in grade display order")
	assert.Equal(t, yarntxweight.GradeA9, rules[1].Grade)
	assert.Equal(t, yarntxweight.ModeMultiply, rules[1].Mode, "mode normalized")
	assert.Equal(t, yarntxweight.GradeC, rules[2].Grade)
	assert.InDelta(t, 99.5, rules[0].Apply(100), 1e-12)
	assert.False(t, e.IsDeleted())
}

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(in *yarntxweightgroup.Input)
		by   string
		want error
	}{
		{"empty code", func(in *yarntxweightgroup.Input) { in.Code = " " }, "u", yarntxweightgroup.ErrInvalidCode},
		{"bad code", func(in *yarntxweightgroup.Input) { in.Code = "1TTY" }, "u", yarntxweightgroup.ErrInvalidCode},
		{"long code", func(in *yarntxweightgroup.Input) { in.Code = strings.Repeat("A", 31) }, "u", yarntxweightgroup.ErrInvalidCode},
		{"empty name", func(in *yarntxweightgroup.Input) { in.Name = "" }, "u", yarntxweightgroup.ErrInvalidName},
		{"long name", func(in *yarntxweightgroup.Input) { in.Name = strings.Repeat("n", 101) }, "u", yarntxweightgroup.ErrInvalidName},
		{"long description", func(in *yarntxweightgroup.Input) { in.Description = strings.Repeat("d", 201) }, "u", yarntxweightgroup.ErrDescriptionTooLong},
		{"no types", func(in *yarntxweightgroup.Input) { in.ProductTypeIDs = nil }, "u", yarntxweightgroup.ErrNoProductTypes},
		{"zero type", func(in *yarntxweightgroup.Input) { in.ProductTypeIDs = []int32{1, 0} }, "u", yarntxweightgroup.ErrInvalidProductType},
		{"duplicate type", func(in *yarntxweightgroup.Input) { in.ProductTypeIDs = []int32{1, 2, 1} }, "u", yarntxweightgroup.ErrDuplicateProductType},
		{"no rules", func(in *yarntxweightgroup.Input) { in.Rules = nil }, "u", yarntxweightgroup.ErrInvalidRuleCount},
		{"six rules", func(in *yarntxweightgroup.Input) {
			in.Rules = make([]yarntxweightgroup.Rule, 6)
		}, "u", yarntxweightgroup.ErrInvalidRuleCount},
		{"duplicate grade", func(in *yarntxweightgroup.Input) {
			in.Rules = []yarntxweightgroup.Rule{
				{Grade: yarntxweight.GradeA, Mode: yarntxweight.ModeFixed, Value: 1},
				{Grade: "a", Mode: yarntxweight.ModeLessBy, Value: 2},
			}
		}, "u", yarntxweightgroup.ErrDuplicateGrade},
		{"bad grade", func(in *yarntxweightgroup.Input) {
			in.Rules = []yarntxweightgroup.Rule{{Grade: "Z", Mode: yarntxweight.ModeFixed}}
		}, "u", yarntxweight.ErrInvalidGrade},
		{"bad mode", func(in *yarntxweightgroup.Input) {
			in.Rules = []yarntxweightgroup.Rule{{Grade: yarntxweight.GradeB, Mode: "PLUS"}}
		}, "u", yarntxweight.ErrInvalidMode},
		{"nan value", func(in *yarntxweightgroup.Input) {
			in.Rules = []yarntxweightgroup.Rule{{Grade: yarntxweight.GradeB, Mode: yarntxweight.ModeFixed, Value: math.Inf(1)}}
		}, "u", yarntxweight.ErrInvalidValue},
		{"long rule description", func(in *yarntxweightgroup.Input) {
			in.Rules = []yarntxweightgroup.Rule{{Grade: yarntxweight.GradeB, Mode: yarntxweight.ModeFixed, Description: strings.Repeat("x", 201)}}
		}, "u", yarntxweight.ErrDescriptionTooLong},
		{"empty created by", func(*yarntxweightgroup.Input) {}, "", yarntxweightgroup.ErrEmptyCreatedBy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mut(&in)
			_, err := yarntxweightgroup.New(in, tc.by)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestUpdate_ReplacesSetAndRules(t *testing.T) {
	e, err := yarntxweightgroup.New(validInput(), "admin")
	require.NoError(t, err)

	err = e.Update(yarntxweightgroup.Input{
		Code: "DTY", Name: "Draw Textured", ProductTypeIDs: []int32{9, 11},
		Rules: []yarntxweightgroup.Rule{{Grade: yarntxweight.GradeB, Mode: yarntxweight.ModeFixed, Value: 2.5}},
	}, "editor")
	require.NoError(t, err)
	assert.Equal(t, "DTY", e.Code())
	assert.Equal(t, []int32{9, 11}, e.ProductTypeIDs(), "type set replaced, not merged")
	require.Len(t, e.Rules(), 1, "rules replaced, not merged")
	assert.Equal(t, yarntxweight.GradeB, e.Rules()[0].Grade)
	require.NotNil(t, e.UpdatedBy())
	assert.Equal(t, "editor", *e.UpdatedBy())

	bad := validInput()
	bad.ProductTypeIDs = nil
	assert.ErrorIs(t, e.Update(bad, "editor"), yarntxweightgroup.ErrNoProductTypes)
	assert.Equal(t, "DTY", e.Code(), "failed update must not mutate")
	assert.Equal(t, []int32{9, 11}, e.ProductTypeIDs())
}

func TestSoftDelete(t *testing.T) {
	e, err := yarntxweightgroup.New(validInput(), "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.True(t, e.IsDeleted())
	assert.ErrorIs(t, e.SoftDelete("admin"), yarntxweightgroup.ErrAlreadyDeleted)
	assert.ErrorIs(t, e.Update(validInput(), "admin"), yarntxweightgroup.ErrAlreadyDeleted)
}

func TestAccessorsReturnCopies(t *testing.T) {
	e, err := yarntxweightgroup.New(validInput(), "admin")
	require.NoError(t, err)
	pts := e.ProductTypes()
	pts[0].ID = 999
	rules := e.Rules()
	rules[0].Value = 999
	assert.Equal(t, int32(3), e.ProductTypes()[0].ID)
	assert.InDelta(t, 0.5, e.Rules()[0].Value, 1e-12)
}

func TestProductTypeConflictError(t *testing.T) {
	err := &yarntxweightgroup.ProductTypeConflictError{Conflicts: []yarntxweightgroup.ProductTypeConflict{
		{ProductTypeID: 3, ProductTypeCode: "PTY", GroupCode: "DTY"},
		{ProductTypeID: 4, ProductTypeCode: "TTS", GroupCode: "TTY"},
	}}
	assert.True(t, errors.Is(err, yarntxweightgroup.ErrProductTypeAlreadyMapped))
	assert.Equal(t,
		"product type PTY is already assigned to tx weight group DTY; product type TTS is already assigned to tx weight group TTY",
		err.Error())
}

func TestListFilterValidate(t *testing.T) {
	f := yarntxweightgroup.ListFilter{PageSize: 500}
	f.Validate()
	assert.Equal(t, 1, f.Page)
	assert.Equal(t, 100, f.PageSize)
	assert.Equal(t, yarntxweightgroup.SortByCode, f.SortBy)
	assert.Equal(t, "asc", f.SortOrder)
	f.Page = 3
	assert.Equal(t, 200, f.Offset())
}
