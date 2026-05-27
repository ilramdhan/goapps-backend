// Package formula provides domain logic for Formula management.
package formula

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Code Value Object Tests
// =============================================================================

func TestNewCode_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "COST_ELEC", "COST_ELEC"},
		{"single letter", "A", "A"},
		{"with numbers", "FORMULA1", "FORMULA1"},
		{"with underscore", "COST_ELEC_STD", "COST_ELEC_STD"},
		{"trimmed", "  COST_ELEC  ", "COST_ELEC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := NewCode(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, code.String())
		})
	}
}

func TestNewCode_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"empty", "", ErrEmptyCode},
		{"whitespace only", "   ", ErrEmptyCode},
		{"lowercase", "cost_elec", ErrInvalidCodeFormat},
		{"starts with number", "1COST", ErrInvalidCodeFormat},
		{"starts with underscore", "_COST", ErrInvalidCodeFormat},
		{"contains space", "COST ELEC", ErrInvalidCodeFormat},
		{"too long", strings.Repeat("A", 51), ErrCodeTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCode(tt.input)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// =============================================================================
// Type Value Object Tests
// =============================================================================

func TestNewType_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CALCULATION", "CALCULATION"},
		{"SQL_QUERY", "SQL_QUERY"},
		{"CONSTANT", "CONSTANT"},
		{"  calculation  ", "CALCULATION"},
		{"sql_query", "SQL_QUERY"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ft, err := NewType(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, ft.String())
			assert.True(t, ft.IsValid())
		})
	}
}

func TestNewType_Invalid(t *testing.T) {
	tests := []string{"", "INVALID", "CALC", "SQL"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := NewType(input)
			assert.ErrorIs(t, err, ErrInvalidFormulaType)
		})
	}
}

// =============================================================================
// NewFormula Tests
// =============================================================================

func TestNewFormula_Success(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	resultID := uuid.New()
	inputIDs := []uuid.UUID{uuid.New(), uuid.New()}

	f, err := NewFormula(code, "Electricity Cost", TypeCalculation,
		"ELEC_CONSUMPTION * ELEC_RATE", resultID, inputIDs, "Test formula", "admin")

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, f.ID())
	assert.Equal(t, "COST_ELEC", f.Code().String())
	assert.Equal(t, "Electricity Cost", f.Name())
	assert.Equal(t, "CALCULATION", f.FormulaType().String())
	assert.Equal(t, "ELEC_CONSUMPTION * ELEC_RATE", f.Expression())
	assert.Equal(t, resultID, f.ResultParamID())
	assert.Equal(t, "Test formula", f.Description())
	assert.Equal(t, 1, f.Version())
	assert.True(t, f.IsActive())
	assert.Equal(t, "admin", f.CreatedBy())
	assert.Len(t, f.InputParams(), 2)
	assert.Nil(t, f.UpdatedAt())
	assert.Nil(t, f.DeletedAt())
	assert.False(t, f.IsDeleted())
}

func TestNewFormula_EmptyName(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	_, err := NewFormula(code, "", TypeCalculation, "expr", uuid.New(), nil, "", "admin")
	assert.ErrorIs(t, err, ErrEmptyName)
}

func TestNewFormula_NameTooLong(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	_, err := NewFormula(code, strings.Repeat("a", 201), TypeCalculation, "expr", uuid.New(), nil, "", "admin")
	assert.ErrorIs(t, err, ErrNameTooLong)
}

func TestNewFormula_EmptyExpression(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	_, err := NewFormula(code, "Test", TypeCalculation, "", uuid.New(), nil, "", "admin")
	assert.ErrorIs(t, err, ErrEmptyExpression)
}

func TestNewFormula_ExpressionTooLong(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	_, err := NewFormula(code, "Test", TypeCalculation, strings.Repeat("x", 5001), uuid.New(), nil, "", "admin")
	assert.ErrorIs(t, err, ErrExpressionTooLong)
}

func TestNewFormula_DescriptionTooLong(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	_, err := NewFormula(code, "Test", TypeCalculation, "expr", uuid.New(), nil, strings.Repeat("x", 1001), "admin")
	assert.ErrorIs(t, err, ErrDescriptionTooLong)
}

func TestNewFormula_EmptyCreatedBy(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	_, err := NewFormula(code, "Test", TypeCalculation, "expr", uuid.New(), nil, "", "")
	assert.ErrorIs(t, err, ErrEmptyCreatedBy)
}

func TestNewFormula_CircularReference(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	resultID := uuid.New()
	inputIDs := []uuid.UUID{uuid.New(), resultID} // result param also in input

	_, err := NewFormula(code, "Test", TypeCalculation, "expr", resultID, inputIDs, "", "admin")
	assert.ErrorIs(t, err, ErrCircularReference)
}

func TestNewFormula_DuplicateInputParam(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	inputID := uuid.New()
	inputIDs := []uuid.UUID{inputID, inputID} // duplicate

	_, err := NewFormula(code, "Test", TypeCalculation, "expr", uuid.New(), inputIDs, "", "admin")
	assert.ErrorIs(t, err, ErrDuplicateInputParam)
}

func TestNewFormula_NoInputParams(t *testing.T) {
	code, _ := NewCode("CONSTANT_VAL")
	f, err := NewFormula(code, "Constant", TypeConstant, "42", uuid.New(), nil, "", "admin")
	require.NoError(t, err)
	assert.Empty(t, f.InputParams())
}

// =============================================================================
// Update Tests
// =============================================================================

func TestFormula_Update_Success(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Old Name", TypeCalculation, "old_expr", uuid.New(), nil, "", "admin")

	newName := "New Name"
	newExpr := "new_expr"
	newDesc := "new desc"
	active := false

	err := f.Update(&newName, nil, &newExpr, nil, nil, &newDesc, &active, "editor")
	require.NoError(t, err)

	assert.Equal(t, "New Name", f.Name())
	assert.Equal(t, "new_expr", f.Expression())
	assert.Equal(t, "new desc", f.Description())
	assert.False(t, f.IsActive())
	assert.Equal(t, 2, f.Version())
	assert.NotNil(t, f.UpdatedAt())
	assert.Equal(t, "editor", *f.UpdatedBy())
}

func TestFormula_Update_NilFields_NoChange(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Original", TypeCalculation, "expr", uuid.New(), nil, "desc", "admin")

	err := f.Update(nil, nil, nil, nil, nil, nil, nil, "editor")
	require.NoError(t, err)

	assert.Equal(t, "Original", f.Name())
	assert.Equal(t, "expr", f.Expression())
	assert.Equal(t, "desc", f.Description())
	assert.Equal(t, 2, f.Version()) // version still increments
}

func TestFormula_Update_InvalidName(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", uuid.New(), nil, "", "admin")

	empty := ""
	err := f.Update(&empty, nil, nil, nil, nil, nil, nil, "editor")
	assert.ErrorIs(t, err, ErrEmptyName)
}

func TestFormula_Update_InvalidFormulaType(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", uuid.New(), nil, "", "admin")

	invalid := Type{value: "INVALID"}
	err := f.Update(nil, &invalid, nil, nil, nil, nil, nil, "editor")
	assert.ErrorIs(t, err, ErrInvalidFormulaType)
}

func TestFormula_Update_WithInputParams(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	resultID := uuid.New()
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", resultID, nil, "", "admin")

	newInputIDs := []uuid.UUID{uuid.New(), uuid.New()}
	err := f.Update(nil, nil, nil, nil, newInputIDs, nil, nil, "editor")
	require.NoError(t, err)
	assert.Len(t, f.InputParams(), 2)
}

func TestFormula_Update_CircularReference(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	resultID := uuid.New()
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", resultID, nil, "", "admin")

	inputIDs := []uuid.UUID{resultID} // circular
	err := f.Update(nil, nil, nil, nil, inputIDs, nil, nil, "editor")
	assert.ErrorIs(t, err, ErrCircularReference)
}

func TestFormula_Update_AlreadyDeleted(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", uuid.New(), nil, "", "admin")
	_ = f.SoftDelete("admin")

	name := "New"
	err := f.Update(&name, nil, nil, nil, nil, nil, nil, "editor")
	assert.ErrorIs(t, err, ErrAlreadyDeleted)
}

// =============================================================================
// SoftDelete Tests
// =============================================================================

func TestFormula_SoftDelete_Success(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", uuid.New(), nil, "", "admin")

	err := f.SoftDelete("admin")
	require.NoError(t, err)

	assert.True(t, f.IsDeleted())
	assert.NotNil(t, f.DeletedAt())
	assert.Equal(t, "admin", *f.DeletedBy())
	assert.False(t, f.IsActive())
}

func TestFormula_SoftDelete_AlreadyDeleted(t *testing.T) {
	code, _ := NewCode("COST_ELEC")
	f, _ := NewFormula(code, "Name", TypeCalculation, "expr", uuid.New(), nil, "", "admin")
	_ = f.SoftDelete("admin")

	err := f.SoftDelete("admin")
	assert.ErrorIs(t, err, ErrAlreadyDeleted)
}

// =============================================================================
// Param Tests
// =============================================================================

func TestNewParam(t *testing.T) {
	paramID := uuid.New()
	fp := NewParam(paramID, 1)

	assert.NotEqual(t, uuid.Nil, fp.ID())
	assert.Equal(t, paramID, fp.ParamID())
	assert.Equal(t, 1, fp.SortOrder())
	assert.Empty(t, fp.ParamCode())
	assert.Empty(t, fp.ParamName())
}

func TestReconstructParam(t *testing.T) {
	id := uuid.New()
	paramID := uuid.New()
	fp := ReconstructParam(id, paramID, "ELEC_RATE", "Electricity Rate", 3)

	assert.Equal(t, id, fp.ID())
	assert.Equal(t, paramID, fp.ParamID())
	assert.Equal(t, "ELEC_RATE", fp.ParamCode())
	assert.Equal(t, "Electricity Rate", fp.ParamName())
	assert.Equal(t, 3, fp.SortOrder())
}

// =============================================================================
// ListFilter Tests
// =============================================================================

func TestNewListFilter(t *testing.T) {
	f := NewListFilter()
	assert.Equal(t, 1, f.Page)
	assert.Equal(t, 10, f.PageSize)
	assert.Equal(t, "code", f.SortBy)
	assert.Equal(t, "asc", f.SortOrder)
}

func TestListFilter_Validate(t *testing.T) {
	tests := []struct {
		name     string
		input    ListFilter
		wantPage int
		wantSize int
	}{
		{"negative page", ListFilter{Page: -1, PageSize: 10}, 1, 10},
		{"zero page size", ListFilter{Page: 1, PageSize: 0}, 1, 10},
		{"over max page size", ListFilter{Page: 1, PageSize: 200}, 1, 100},
		{"valid", ListFilter{Page: 3, PageSize: 25}, 3, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Validate()
			assert.Equal(t, tt.wantPage, tt.input.Page)
			assert.Equal(t, tt.wantSize, tt.input.PageSize)
		})
	}
}

func TestListFilter_Offset(t *testing.T) {
	f := ListFilter{Page: 3, PageSize: 10}
	assert.Equal(t, 20, f.Offset())
}
