// Package rmcategory provides domain layer tests for RMCategory entity.
package rmcategory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// =============================================================================
// Value Object: Code
// =============================================================================

func TestNewCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "valid code - simple", input: "CHIP", want: "CHIP"},
		{name: "valid code - with underscore", input: "OIL_PALM", want: "OIL_PALM"},
		{name: "valid code - with numbers", input: "CAT123", want: "CAT123"},
		{name: "valid code - normalizes to uppercase", input: "chip", want: "CHIP"},
		{name: "valid code - trims spaces", input: "  CHIP  ", want: "CHIP"},
		{name: "invalid - empty", input: "", wantErr: rmcategory.ErrEmptyCode},
		{name: "invalid - whitespace only", input: "   ", wantErr: rmcategory.ErrEmptyCode},
		{name: "invalid - starts with number", input: "1CHIP", wantErr: rmcategory.ErrInvalidCodeFormat},
		{name: "invalid - special characters", input: "CHIP@#", wantErr: rmcategory.ErrInvalidCodeFormat},
		{name: "invalid - too long", input: strings.Repeat("A", 21), wantErr: rmcategory.ErrCodeTooLong},
		{name: "valid - max length", input: strings.Repeat("A", 20), want: strings.Repeat("A", 20)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := rmcategory.NewCode(tt.input)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, code.String())
			}
		})
	}
}

func TestCode_Equal(t *testing.T) {
	code1, _ := rmcategory.NewCode("CHIP")
	code2, _ := rmcategory.NewCode("CHIP")
	code3, _ := rmcategory.NewCode("OIL")

	assert.True(t, code1.Equal(code2))
	assert.False(t, code1.Equal(code3))
}

func TestCode_IsEmpty(t *testing.T) {
	var empty rmcategory.Code
	assert.True(t, empty.IsEmpty())

	code, _ := rmcategory.NewCode("CHIP")
	assert.False(t, code.IsEmpty())
}

// =============================================================================
// Entity: RMCategory Constructor
// =============================================================================

func TestNewRMCategory(t *testing.T) {
	code, _ := rmcategory.NewCode("CHIP")

	tests := []struct {
		name        string
		code        rmcategory.Code
		catName     string
		description string
		createdBy   string
		wantErr     error
	}{
		{
			name: "valid - all fields", code: code, catName: "Chips",
			description: "Raw material chips", createdBy: "admin",
		},
		{
			name: "valid - empty description", code: code, catName: "Chips",
			description: "", createdBy: "admin",
		},
		{
			name: "invalid - empty name", code: code, catName: "",
			createdBy: "admin", wantErr: rmcategory.ErrEmptyName,
		},
		{
			name: "invalid - name too long", code: code,
			catName: strings.Repeat("A", 101), createdBy: "admin",
			wantErr: rmcategory.ErrNameTooLong,
		},
		{
			name: "invalid - empty createdBy", code: code, catName: "Chips",
			createdBy: "", wantErr: rmcategory.ErrEmptyCreatedBy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := rmcategory.NewRMCategory(tt.code, tt.catName, tt.description, tt.createdBy)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, entity)
			} else {
				require.NoError(t, err)
				require.NotNil(t, entity)
				assert.NotEqual(t, uuid.Nil, entity.ID())
				assert.Equal(t, tt.code.String(), entity.Code().String())
				assert.Equal(t, tt.catName, entity.Name())
				assert.Equal(t, tt.description, entity.Description())
				assert.True(t, entity.IsActive())
				assert.Equal(t, tt.createdBy, entity.CreatedBy())
				assert.False(t, entity.CreatedAt().IsZero())
				assert.Nil(t, entity.UpdatedAt())
				assert.Nil(t, entity.UpdatedBy())
				assert.Nil(t, entity.DeletedAt())
				assert.Nil(t, entity.DeletedBy())
				assert.False(t, entity.IsDeleted())
			}
		})
	}
}

// =============================================================================
// Entity: ReconstructRMCategory
// =============================================================================

func TestReconstructRMCategory(t *testing.T) {
	id := uuid.New()
	code, _ := rmcategory.NewCode("OIL")
	now := time.Now()
	updatedBy := "editor"

	entity := rmcategory.ReconstructRMCategory(
		id, code, "Oil", "Various types of oil", true,
		now, "admin", &now, &updatedBy, nil, nil,
	)

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID())
	assert.Equal(t, "OIL", entity.Code().String())
	assert.Equal(t, "Oil", entity.Name())
	assert.Equal(t, "Various types of oil", entity.Description())
	assert.True(t, entity.IsActive())
	assert.Equal(t, "admin", entity.CreatedBy())
	assert.NotNil(t, entity.UpdatedAt())
	assert.Equal(t, "editor", *entity.UpdatedBy())
	assert.False(t, entity.IsDeleted())
}

// =============================================================================
// Entity: Update
// =============================================================================

func TestRMCategory_Update(t *testing.T) {
	newEntity := func() *rmcategory.RMCategory {
		code, _ := rmcategory.NewCode("CHIP")
		entity, _ := rmcategory.NewRMCategory(code, "Chips", "Description", "admin")
		return entity
	}

	t.Run("update name only", func(t *testing.T) {
		entity := newEntity()
		newName := "Wood Chips"
		err := entity.Update(&newName, nil, nil, "editor")
		require.NoError(t, err)
		assert.Equal(t, "Wood Chips", entity.Name())
		assert.Equal(t, "Description", entity.Description())
		assert.NotNil(t, entity.UpdatedAt())
		assert.Equal(t, "editor", *entity.UpdatedBy())
	})

	t.Run("update description only", func(t *testing.T) {
		entity := newEntity()
		newDesc := "Updated description"
		err := entity.Update(nil, &newDesc, nil, "editor")
		require.NoError(t, err)
		assert.Equal(t, "Updated description", entity.Description())
	})

	t.Run("update isActive only", func(t *testing.T) {
		entity := newEntity()
		inactive := false
		err := entity.Update(nil, nil, &inactive, "editor")
		require.NoError(t, err)
		assert.False(t, entity.IsActive())
	})

	t.Run("update all fields", func(t *testing.T) {
		entity := newEntity()
		name := "New Name"
		desc := "New Desc"
		active := false
		err := entity.Update(&name, &desc, &active, "editor")
		require.NoError(t, err)
		assert.Equal(t, "New Name", entity.Name())
		assert.Equal(t, "New Desc", entity.Description())
		assert.False(t, entity.IsActive())
	})

	t.Run("error - empty name", func(t *testing.T) {
		entity := newEntity()
		emptyName := ""
		err := entity.Update(&emptyName, nil, nil, "editor")
		assert.ErrorIs(t, err, rmcategory.ErrEmptyName)
	})

	t.Run("error - name too long", func(t *testing.T) {
		entity := newEntity()
		longName := strings.Repeat("A", 101)
		err := entity.Update(&longName, nil, nil, "editor")
		assert.ErrorIs(t, err, rmcategory.ErrNameTooLong)
	})

	t.Run("error - already deleted", func(t *testing.T) {
		entity := newEntity()
		_ = entity.SoftDelete("admin")
		name := "New Name"
		err := entity.Update(&name, nil, nil, "editor")
		assert.ErrorIs(t, err, rmcategory.ErrAlreadyDeleted)
	})
}

// =============================================================================
// Entity: SoftDelete
// =============================================================================

func TestRMCategory_SoftDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		code, _ := rmcategory.NewCode("CHIP")
		entity, _ := rmcategory.NewRMCategory(code, "Chips", "", "admin")

		err := entity.SoftDelete("admin")
		require.NoError(t, err)
		assert.True(t, entity.IsDeleted())
		assert.NotNil(t, entity.DeletedAt())
		assert.Equal(t, "admin", *entity.DeletedBy())
		assert.False(t, entity.IsActive())
	})

	t.Run("error - already deleted", func(t *testing.T) {
		code, _ := rmcategory.NewCode("CHIP")
		entity, _ := rmcategory.NewRMCategory(code, "Chips", "", "admin")
		_ = entity.SoftDelete("admin")

		err := entity.SoftDelete("admin")
		assert.ErrorIs(t, err, rmcategory.ErrAlreadyDeleted)
	})
}

// =============================================================================
// Entity: Activate / Deactivate
// =============================================================================

func TestRMCategory_Activate(t *testing.T) {
	code, _ := rmcategory.NewCode("CHIP")
	entity, _ := rmcategory.NewRMCategory(code, "Chips", "", "admin")
	inactive := false
	_ = entity.Update(nil, nil, &inactive, "admin")
	assert.False(t, entity.IsActive())

	err := entity.Activate("editor")
	require.NoError(t, err)
	assert.True(t, entity.IsActive())
}

func TestRMCategory_Deactivate(t *testing.T) {
	code, _ := rmcategory.NewCode("CHIP")
	entity, _ := rmcategory.NewRMCategory(code, "Chips", "", "admin")
	assert.True(t, entity.IsActive())

	err := entity.Deactivate("editor")
	require.NoError(t, err)
	assert.False(t, entity.IsActive())
}

func TestRMCategory_ActivateDeactivate_ErrorWhenDeleted(t *testing.T) {
	code, _ := rmcategory.NewCode("CHIP")
	entity, _ := rmcategory.NewRMCategory(code, "Chips", "", "admin")
	_ = entity.SoftDelete("admin")

	assert.ErrorIs(t, entity.Activate("editor"), rmcategory.ErrAlreadyDeleted)
	assert.ErrorIs(t, entity.Deactivate("editor"), rmcategory.ErrAlreadyDeleted)
}
