// Package uom provides domain layer tests for UOM entity.
package uom_test

import (
	"strings"
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

func TestNewCategory(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid - WEIGHT", input: "WEIGHT", wantErr: false},
		{name: "valid - LENGTH", input: "LENGTH", wantErr: false},
		{name: "valid - VOLUME", input: "VOLUME", wantErr: false},
		{name: "valid - QUANTITY", input: "QUANTITY", wantErr: false},
		{name: "valid - lowercase (auto uppercase)", input: "weight", wantErr: false},
		{name: "invalid - empty", input: "", wantErr: true},
		{name: "invalid - unknown", input: "UNKNOWN", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, err := uom.NewCategory(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// All valid categories should be uppercase
				assert.Equal(t, strings.ToUpper(tt.input), cat.String())
			}
		})
	}
}

func TestNewUOM(t *testing.T) {
	t.Run("valid UOM creation", func(t *testing.T) {
		code, _ := uom.NewCode("KG")
		category, _ := uom.NewCategory("WEIGHT")

		entity, err := uom.NewUOM(code, "Kilogram", category, "Unit of weight", "admin")

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, entity.ID())
		assert.Equal(t, "KG", entity.Code().String())
		assert.Equal(t, "Kilogram", entity.Name())
		assert.Equal(t, "WEIGHT", entity.Category().String())
		assert.Equal(t, "Unit of weight", entity.Description())
		assert.True(t, entity.IsActive())
		assert.Equal(t, "admin", entity.CreatedBy())
		assert.False(t, entity.CreatedAt().IsZero())
		assert.Nil(t, entity.UpdatedAt())
		assert.Nil(t, entity.UpdatedBy())
	})

	t.Run("invalid - empty name", func(t *testing.T) {
		code, _ := uom.NewCode("KG")
		category, _ := uom.NewCategory("WEIGHT")

		_, err := uom.NewUOM(code, "", category, "Description", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrEmptyName)
	})

	t.Run("invalid - name too long", func(t *testing.T) {
		code, _ := uom.NewCode("KG")
		category, _ := uom.NewCategory("WEIGHT")
		longName := string(make([]byte, 101)) // 101 characters

		_, err := uom.NewUOM(code, longName, category, "Description", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrNameTooLong)
	})

	t.Run("invalid - empty created by", func(t *testing.T) {
		code, _ := uom.NewCode("KG")
		category, _ := uom.NewCategory("WEIGHT")

		_, err := uom.NewUOM(code, "Kilogram", category, "Description", "")

		assert.Error(t, err)
		assert.ErrorIs(t, err, uom.ErrEmptyCreatedBy)
	})
}

func TestUOM_Update(t *testing.T) {
	// Create a valid entity first
	code, _ := uom.NewCode("KG")
	category, _ := uom.NewCategory("WEIGHT")
	entity, _ := uom.NewUOM(code, "Kilogram", category, "Old description", "admin")

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
		newCat, _ := uom.NewCategory("LENGTH")
		err := entity.Update(nil, &newCat, nil, nil, "editor2")

		require.NoError(t, err)
		assert.Equal(t, "LENGTH", entity.Category().String())
		assert.Equal(t, "editor2", *entity.UpdatedBy())
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
	category, _ := uom.NewCategory("VOLUME")
	createdAt := time.Now().Add(-24 * time.Hour)
	updatedAt := time.Now()
	updatedBy := "updater"

	entity := uom.ReconstructUOM(
		id,
		code,
		"Liter",
		category,
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
	assert.Equal(t, "VOLUME", entity.Category().String())
	assert.Equal(t, "Volume unit", entity.Description())
	assert.True(t, entity.IsActive())
	assert.Equal(t, createdAt, entity.CreatedAt())
	assert.Equal(t, "creator", entity.CreatedBy())
	assert.NotNil(t, entity.UpdatedAt())
	assert.Equal(t, updatedAt, *entity.UpdatedAt())
	assert.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, "updater", *entity.UpdatedBy())
}

func TestCategory_IsValid(t *testing.T) {
	validCategories := []string{"WEIGHT", "LENGTH", "VOLUME", "QUANTITY"}

	for _, catStr := range validCategories {
		cat, err := uom.NewCategory(catStr)
		assert.NoError(t, err)
		assert.True(t, cat.IsValid())
	}
}
