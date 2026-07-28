package wastecategory_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/wastecategory"
)

func mustArea(t *testing.T, s string) area.Area {
	t.Helper()
	a, err := area.New(s)
	require.NoError(t, err)
	return a
}

func strptr(s string) *string { return &s }

func TestNewCategory(t *testing.T) {
	t.Parallel()

	gradeB := strptr("B")

	tests := []struct {
		name        string
		areaStr     string
		wasteType   string
		code        string
		catName     string
		gradeTarget *string
		createdBy   string
		wantErr     error
	}{
		{"valid waste", "TXT", "WASTE", "SPINNING", "Spinning waste", nil, "tester", nil},
		{"valid downgrade", "TXT", "DOWNGRADE", "DTY", "Downgrade DTY", gradeB, "tester", nil},
		{"invalid type", "TXT", "SCRAP", "X", "X", nil, "tester", wastecategory.ErrInvalidType},
		{"empty code", "TXT", "WASTE", "", "X", nil, "tester", wastecategory.ErrEmptyCode},
		{"code too long", "TXT", "WASTE", strings.Repeat("x", 31), "X", nil, "tester", wastecategory.ErrCodeTooLong},
		{"empty name", "TXT", "WASTE", "SPINNING", "", nil, "tester", wastecategory.ErrEmptyName},
		{"downgrade without grade", "TXT", "DOWNGRADE", "DTY", "X", nil, "tester", wastecategory.ErrGradeTargetRequired},
		{"empty created_by", "TXT", "WASTE", "SPINNING", "X", nil, "", wastecategory.ErrEmptyCreatedBy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity, err := wastecategory.NewCategory(
				mustArea(t, tt.areaStr), tt.wasteType, tt.code, tt.catName, tt.gradeTarget, 0, tt.createdBy,
			)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, entity)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, entity)
			assert.Equal(t, tt.wasteType, entity.Type())
			assert.Equal(t, tt.code, entity.Code())
		})
	}
}

func TestNewCategory_WasteBlanksGradeTarget(t *testing.T) {
	t.Parallel()
	entity, err := wastecategory.NewCategory(mustArea(t, "TXT"), "WASTE", "SPINNING", "Spinning", strptr("B"), 0, "tester")
	require.NoError(t, err)
	assert.Nil(t, entity.GradeTarget(), "WASTE type must not carry a grade target")
}

func TestNewCategory_InvalidArea(t *testing.T) {
	t.Parallel()
	entity, err := wastecategory.NewCategory(area.Area{}, "WASTE", "SPINNING", "Spinning", nil, 0, "tester")
	assert.ErrorIs(t, err, wastecategory.ErrInvalidArea)
	assert.Nil(t, entity)
}

func TestIsValidType(t *testing.T) {
	t.Parallel()
	assert.True(t, wastecategory.IsValidType("WASTE"))
	assert.True(t, wastecategory.IsValidType("DOWNGRADE"))
	assert.False(t, wastecategory.IsValidType("waste"))
}
