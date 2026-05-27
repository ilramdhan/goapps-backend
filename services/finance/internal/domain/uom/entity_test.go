// Package uom provides domain layer tests for UOM entity.
package uom_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

func TestNewCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType error
	}{
		{
			name:    "valid code - simple",
			input:   "KG",
			wantErr: false,
		},
		{
			name:    "valid code - with underscore",
			input:   "MTR_SQ",
			wantErr: false,
		},
		{
			name:    "valid code - with numbers",
			input:   "UNIT123",
			wantErr: false,
		},
		{
			name:    "invalid - empty",
			input:   "",
			wantErr: true,
			errType: uom.ErrEmptyCode,
		},
		{
			name:    "invalid - lowercase",
			input:   "kg",
			wantErr: true,
			errType: uom.ErrInvalidCodeFormat,
		},
		{
			name:    "invalid - starts with number",
			input:   "1KG",
			wantErr: true,
			errType: uom.ErrInvalidCodeFormat,
		},
		{
			name:    "invalid - special characters",
			input:   "KG@#",
			wantErr: true,
			errType: uom.ErrInvalidCodeFormat,
		},
		{
			name:    "invalid - too long",
			input:   "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
			wantErr: true,
			errType: uom.ErrCodeTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := uom.NewCode(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.input, code.String())
			}
		})
	}
}

func TestCategoryInfo(t *testing.T) {
	t.Run("create category info", func(t *testing.T) {
		id := uuid.New()
		info := uom.NewCategoryInfo(id, "WEIGHT", "Weight")

		assert.Equal(t, id, info.ID())
		assert.Equal(t, "WEIGHT", info.Code())
		assert.Equal(t, "Weight", info.Name())
	})

	t.Run("zero value category info", func(t *testing.T) {
		info := uom.CategoryInfo{}
		assert.Equal(t, uuid.Nil, info.ID())
		assert.Equal(t, "", info.Code())
		assert.Equal(t, "", info.Name())
	})
}

func TestNewUOM(t *testing.T) {
	categoryID := uuid.New()

	t.Run("valid UOM creation", func(t *testing.T) {
		code, _ := uom.NewCode("KG")

		entity, err := uom.NewUOM(code, "Kilogram", categoryID, "Unit of weight", "admin")

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, entity.ID())
		assert.Equal(t, "KG", entity.Code().String())
		assert.Equal(t, "Kilogram", entity.Name())
		assert.Equal(t, categoryID, entity.CategoryID())
		assert.Equal(t, "Unit of weight", entity.Description())
		assert.True(t, entity.IsActive())
		assert.Equal(t, "admin", entity.CreatedBy())
		assert.False(t, entity.CreatedAt().IsZero())
		assert.Nil(t, entity.UpdatedAt())
		assert.Nil(t, entity.UpdatedBy())
	})

	t.Run("invalid - empty name", func(t *testing.T) {
		code, _ := uom.NewCode("KG")

		_, err := uom.NewUOM(code, "", categoryID, "Description", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrEmptyName)
	})

	t.Run("invalid - name too long", func(t *testing.T) {
		code, _ := uom.NewCode("KG")
		longName := string(make([]byte, 101)) // 101 characters

		_, err := uom.NewUOM(code, longName, categoryID, "Description", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrNameTooLong)
	})

	t.Run("invalid - empty created by", func(t *testing.T) {
		code, _ := uom.NewCode("KG")

		_, err := uom.NewUOM(code, "Kilogram", categoryID, "Description", "")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrEmptyCreatedBy)
	})

	t.Run("invalid - nil category ID", func(t *testing.T) {
		code, _ := uom.NewCode("KG")

		_, err := uom.NewUOM(code, "Kilogram", uuid.Nil, "Description", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrInvalidCategory)
	})
}

func TestUOM_Update(t *testing.T) {
	// Create a valid entity first
	code, _ := uom.NewCode("KG")
	categoryID := uuid.New()
	entity, _ := uom.NewUOM(code, "Kilogram", categoryID, "Old description", "admin")

	t.Run("update name", func(t *testing.T) {
		newName := "Kilogram Updated"
		err := entity.Update(&newName, nil, nil, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "Kilogram Updated", entity.Name())
		assert.NotNil(t, entity.UpdatedAt())
		assert.NotNil(t, entity.UpdatedBy())
		assert.Equal(t, "editor", *entity.UpdatedBy())
	})

	t.Run("update category", func(t *testing.T) {
		newCatID := uuid.New()
		err := entity.Update(nil, &newCatID, nil, nil, "editor2")

		require.NoError(t, err)
		assert.Equal(t, newCatID, entity.CategoryID())
		assert.Equal(t, "editor2", *entity.UpdatedBy())
	})

	t.Run("update category with nil UUID", func(t *testing.T) {
		nilID := uuid.Nil
		err := entity.Update(nil, &nilID, nil, nil, "editor")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrInvalidCategory)
	})

	t.Run("update description", func(t *testing.T) {
		newDesc := "New description"
		err := entity.Update(nil, nil, &newDesc, nil, "editor3")

		require.NoError(t, err)
		assert.Equal(t, "New description", entity.Description())
	})

	t.Run("update is_active", func(t *testing.T) {
		inactive := false
		err := entity.Update(nil, nil, nil, &inactive, "editor4")

		require.NoError(t, err)
		assert.False(t, entity.IsActive())
	})
}

func TestReconstructUOM(t *testing.T) {
	id := uuid.New()
	code, _ := uom.NewCode("LTR")
	categoryInfo := uom.NewCategoryInfo(uuid.New(), "VOLUME", "Volume")
	createdAt := time.Now().Add(-24 * time.Hour)
	updatedAt := time.Now()
	updatedBy := "updater"

	entity := uom.ReconstructUOM(
		id,
		code,
		"Liter",
		categoryInfo,
		"Volume unit",
		true,
		createdAt,
		"creator",
		&updatedAt,
		&updatedBy,
		nil,
		nil,
	)

	assert.Equal(t, id, entity.ID())
	assert.Equal(t, "LTR", entity.Code().String())
	assert.Equal(t, "Liter", entity.Name())
	assert.Equal(t, categoryInfo.ID(), entity.CategoryID())
	assert.Equal(t, "VOLUME", entity.CategoryInfo().Code())
	assert.Equal(t, "Volume", entity.CategoryInfo().Name())
	assert.Equal(t, "Volume unit", entity.Description())
	assert.True(t, entity.IsActive())
	assert.Equal(t, createdAt, entity.CreatedAt())
	assert.Equal(t, "creator", entity.CreatedBy())
	assert.NotNil(t, entity.UpdatedAt())
	assert.Equal(t, updatedAt, *entity.UpdatedAt())
	assert.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, "updater", *entity.UpdatedBy())
}
