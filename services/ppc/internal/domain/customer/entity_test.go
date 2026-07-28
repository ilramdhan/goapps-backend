package customer_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
)

func strPtr(s string) *string { return &s }

func TestNew_ValidParams_NormalizesCodeAndMarksManual(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{
		Code:      "  dc00594 ",
		Name:      "  PT. BESTOW ATRI PARIS TEKSTIL ",
		ShortName: strPtr("BESTOW"),
		CreatedBy: "planner",
	})
	require.NoError(t, err)
	assert.Equal(t, "DC00594", entity.Code())
	assert.Equal(t, "PT. BESTOW ATRI PARIS TEKSTIL", entity.Name())
	assert.Equal(t, customerdomain.SourceManual, entity.Source())
	assert.True(t, entity.IsActive())
	require.NotNil(t, entity.ShortName())
	assert.Equal(t, "BESTOW", *entity.ShortName())
	assert.Nil(t, entity.TaxNo())
	assert.Nil(t, entity.SyncedAt())
}

func TestNew_BlankOptionalField_StoresNil(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{
		Code:      "DC1",
		Name:      "ACME",
		ShortName: strPtr("   "),
		TaxNo:     strPtr(""),
		CreatedBy: "planner",
	})
	require.NoError(t, err)
	assert.Nil(t, entity.ShortName())
	assert.Nil(t, entity.TaxNo())
}

func TestNew_InvalidParams_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		params  customerdomain.NewParams
		wantErr error
	}{
		{
			name:    "empty code",
			params:  customerdomain.NewParams{Code: "   ", Name: "ACME", CreatedBy: "p"},
			wantErr: customerdomain.ErrEmptyCode,
		},
		{
			name:    "code too long",
			params:  customerdomain.NewParams{Code: strings.Repeat("A", 31), Name: "ACME", CreatedBy: "p"},
			wantErr: customerdomain.ErrCodeTooLong,
		},
		{
			name:    "empty name",
			params:  customerdomain.NewParams{Code: "DC1", Name: "  ", CreatedBy: "p"},
			wantErr: customerdomain.ErrEmptyName,
		},
		{
			name:    "name too long",
			params:  customerdomain.NewParams{Code: "DC1", Name: strings.Repeat("A", 241), CreatedBy: "p"},
			wantErr: customerdomain.ErrNameTooLong,
		},
		{
			name: "short name too long",
			params: customerdomain.NewParams{
				Code: "DC1", Name: "ACME", ShortName: strPtr(strings.Repeat("A", 61)), CreatedBy: "p",
			},
			wantErr: customerdomain.ErrShortNameTooLong,
		},
		{
			name: "tax no too long",
			params: customerdomain.NewParams{
				Code: "DC1", Name: "ACME", TaxNo: strPtr(strings.Repeat("A", 61)), CreatedBy: "p",
			},
			wantErr: customerdomain.ErrTaxNoTooLong,
		},
		{
			name: "parent code too long",
			params: customerdomain.NewParams{
				Code: "DC1", Name: "ACME", ParentCode: strPtr(strings.Repeat("A", 31)), CreatedBy: "p",
			},
			wantErr: customerdomain.ErrParentCodeTooLong,
		},
		{
			name:    "empty created by",
			params:  customerdomain.NewParams{Code: "DC1", Name: "ACME", CreatedBy: " "},
			wantErr: customerdomain.ErrEmptyCreatedBy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := customerdomain.New(tt.params)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, entity)
		})
	}
}

func TestUpdate_PartialFields_LeavesOthersUntouched(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{
		Code: "DC1", Name: "OLD NAME", ShortName: strPtr("OLD"), CreatedBy: "p",
	})
	require.NoError(t, err)

	newName := "NEW NAME"
	inactive := false
	require.NoError(t, entity.Update(customerdomain.UpdateParams{
		Name:      &newName,
		IsActive:  &inactive,
		UpdatedBy: "editor",
	}))

	assert.Equal(t, "NEW NAME", entity.Name())
	require.NotNil(t, entity.ShortName())
	assert.Equal(t, "OLD", *entity.ShortName(), "untouched field must survive a partial update")
	assert.False(t, entity.IsActive())
	require.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, "editor", *entity.UpdatedBy())
	assert.NotNil(t, entity.UpdatedAt())
}

func TestUpdate_CodeIsImmutable(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{Code: "DC1", Name: "ACME", CreatedBy: "p"})
	require.NoError(t, err)
	require.NoError(t, entity.Update(customerdomain.UpdateParams{UpdatedBy: "editor"}))
	assert.Equal(t, "DC1", entity.Code(), "the sync upserts on the code; it must never change")
}

func TestUpdate_InvalidName_ReturnsError(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{Code: "DC1", Name: "ACME", CreatedBy: "p"})
	require.NoError(t, err)

	blank := "  "
	require.ErrorIs(t, entity.Update(customerdomain.UpdateParams{Name: &blank}), customerdomain.ErrEmptyName)
	assert.Equal(t, "ACME", entity.Name(), "a rejected update must not partially apply")
}

func TestUpdate_ParentCodeTooLong_ReturnsError(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{
		Code: "DC1", Name: "ACME", ParentCode: strPtr("GRP"), CreatedBy: "p",
	})
	require.NoError(t, err)

	tooLong := strings.Repeat("A", 31)
	require.ErrorIs(t,
		entity.Update(customerdomain.UpdateParams{ParentCode: &tooLong}),
		customerdomain.ErrParentCodeTooLong,
	)
	require.NotNil(t, entity.ParentCode())
	assert.Equal(t, "GRP", *entity.ParentCode(), "a rejected update must not partially apply")
}

func TestUpdate_BlankOptional_ClearsField(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{
		Code: "DC1", Name: "ACME", ShortName: strPtr("ACM"), CreatedBy: "p",
	})
	require.NoError(t, err)

	require.NoError(t, entity.Update(customerdomain.UpdateParams{ShortName: strPtr("")}))
	assert.Nil(t, entity.ShortName())
}

func TestNormalizeCode(t *testing.T) {
	assert.Equal(t, "DC00594", customerdomain.NormalizeCode(" dc00594 "))
	assert.Empty(t, customerdomain.NormalizeCode("   "))
}

func TestSetID(t *testing.T) {
	entity, err := customerdomain.New(customerdomain.NewParams{Code: "DC1", Name: "ACME", CreatedBy: "p"})
	require.NoError(t, err)
	assert.Zero(t, entity.ID())
	entity.SetID(42)
	assert.Equal(t, int64(42), entity.ID())
}

func TestListFilter_Validate_AppliesDefaults(t *testing.T) {
	tests := []struct {
		name         string
		in           customerdomain.ListFilter
		wantPage     int
		wantPageSize int
		wantSortBy   string
		wantOrder    string
	}{
		{"zero value", customerdomain.ListFilter{}, 1, 10, "code", "asc"},
		{"page below one", customerdomain.ListFilter{Page: -3}, 1, 10, "code", "asc"},
		{"page size over cap", customerdomain.ListFilter{PageSize: 5000}, 1, 100, "code", "asc"},
		{
			"explicit values kept",
			customerdomain.ListFilter{Page: 3, PageSize: 25, SortBy: "name", SortOrder: "desc"},
			3, 25, "name", "desc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.in
			f.Validate()
			assert.Equal(t, tt.wantPage, f.Page)
			assert.Equal(t, tt.wantPageSize, f.PageSize)
			assert.Equal(t, tt.wantSortBy, f.SortBy)
			assert.Equal(t, tt.wantOrder, f.SortOrder)
		})
	}
}

func TestListFilter_Offset(t *testing.T) {
	f := customerdomain.ListFilter{Page: 3, PageSize: 25}
	f.Validate()
	assert.Equal(t, 50, f.Offset())
}
