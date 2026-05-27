// Package uomcategory provides domain layer tests for UOM Category entity.
package uomcategory_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

func TestNewCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "valid - simple", input: "WEIGHT", want: "WEIGHT"},
		{name: "valid - with underscore", input: "RAW_MAT", want: "RAW_MAT"},
		{name: "valid - with numbers", input: "TYPE1", want: "TYPE1"},
		{name: "valid - auto uppercase", input: "weight", want: "WEIGHT"},
		{name: "valid - trimmed", input: "  WEIGHT  ", want: "WEIGHT"},
		{name: "invalid - empty", input: "", wantErr: uomcategory.ErrEmptyCode},
		{name: "invalid - too long", input: "ABCDEFGHIJKLMNOPQRSTU", wantErr: uomcategory.ErrCodeTooLong},
		{name: "invalid - starts with number", input: "1TYPE", wantErr: uomcategory.ErrInvalidCodeFormat},
		{name: "invalid - special chars", input: "TYPE@#", wantErr: uomcategory.ErrInvalidCodeFormat},
		{name: "invalid - spaces", input: "MY TYPE", wantErr: uomcategory.ErrInvalidCodeFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := uomcategory.NewCode(tt.input)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, code.String())
				assert.False(t, code.IsEmpty())
			}
		})
	}
}

func TestCode_Equal(t *testing.T) {
	code1, _ := uomcategory.NewCode("WEIGHT")
	code2, _ := uomcategory.NewCode("WEIGHT")
	code3, _ := uomcategory.NewCode("LENGTH")

	assert.True(t, code1.Equal(code2))
	assert.False(t, code1.Equal(code3))
}

func TestNewCategory(t *testing.T) {
	code, _ := uomcategory.NewCode("WEIGHT")

	t.Run("valid creation", func(t *testing.T) {
		entity, err := uomcategory.NewCategory(code, "Weight", "Unit of weight", "admin")

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, entity.ID())
		assert.Equal(t, "WEIGHT", entity.Code().String())
		assert.Equal(t, "Weight", entity.Name())
		assert.Equal(t, "Unit of weight", entity.Description())
		assert.True(t, entity.IsActive())
		assert.Equal(t, "admin", entity.CreatedBy())
		assert.False(t, entity.CreatedAt().IsZero())
		assert.Nil(t, entity.UpdatedAt())
		assert.Nil(t, entity.UpdatedBy())
		assert.False(t, entity.IsDeleted())
	})

	t.Run("invalid - empty name", func(t *testing.T) {
		_, err := uomcategory.NewCategory(code, "", "desc", "admin")
		assert.ErrorIs(t, err, uomcategory.ErrEmptyName)
	})

	t.Run("invalid - name too long", func(t *testing.T) {
		longName := string(make([]byte, 101))
		_, err := uomcategory.NewCategory(code, longName, "desc", "admin")
		assert.ErrorIs(t, err, uomcategory.ErrNameTooLong)
	})

	t.Run("invalid - empty created by", func(t *testing.T) {
		_, err := uomcategory.NewCategory(code, "Weight", "desc", "")
		assert.ErrorIs(t, err, uomcategory.ErrEmptyCreatedBy)
	})
}

func TestCategory_Update(t *testing.T) {
	code, _ := uomcategory.NewCode("WEIGHT")
	entity, _ := uomcategory.NewCategory(code, "Weight", "Old desc", "admin")

	t.Run("update name", func(t *testing.T) {
		newName := "Weight Updated"
		err := entity.Update(&newName, nil, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "Weight Updated", entity.Name())
		assert.NotNil(t, entity.UpdatedAt())
		assert.Equal(t, "editor", *entity.UpdatedBy())
	})

	t.Run("update description", func(t *testing.T) {
		newDesc := "New description"
		err := entity.Update(nil, &newDesc, nil, "editor2")

		require.NoError(t, err)
		assert.Equal(t, "New description", entity.Description())
	})

	t.Run("update is_active", func(t *testing.T) {
		inactive := false
		err := entity.Update(nil, nil, &inactive, "editor3")

		require.NoError(t, err)
		assert.False(t, entity.IsActive())
	})

	t.Run("update - empty name error", func(t *testing.T) {
		empty := ""
		err := entity.Update(&empty, nil, nil, "editor")
		assert.ErrorIs(t, err, uomcategory.ErrEmptyName)
	})

	t.Run("update - name too long error", func(t *testing.T) {
		longName := string(make([]byte, 101))
		err := entity.Update(&longName, nil, nil, "editor")
		assert.ErrorIs(t, err, uomcategory.ErrNameTooLong)
	})
}

func TestCategory_SoftDelete(t *testing.T) {
	code, _ := uomcategory.NewCode("WEIGHT")
	entity, _ := uomcategory.NewCategory(code, "Weight", "", "admin")

	t.Run("soft delete success", func(t *testing.T) {
		err := entity.SoftDelete("deleter")

		require.NoError(t, err)
		assert.True(t, entity.IsDeleted())
		assert.False(t, entity.IsActive())
		assert.NotNil(t, entity.DeletedAt())
		assert.Equal(t, "deleter", *entity.DeletedBy())
	})

	t.Run("already deleted error", func(t *testing.T) {
		err := entity.SoftDelete("another")
		assert.ErrorIs(t, err, uomcategory.ErrAlreadyDeleted)
	})

	t.Run("update deleted entity error", func(t *testing.T) {
		name := "Should Fail"
		err := entity.Update(&name, nil, nil, "editor")
		assert.ErrorIs(t, err, uomcategory.ErrAlreadyDeleted)
	})
}

func TestCategory_ActivateDeactivate(t *testing.T) {
	code, _ := uomcategory.NewCode("LENGTH")
	entity, _ := uomcategory.NewCategory(code, "Length", "", "admin")

	t.Run("deactivate", func(t *testing.T) {
		err := entity.Deactivate("editor")
		require.NoError(t, err)
		assert.False(t, entity.IsActive())
	})

	t.Run("activate", func(t *testing.T) {
		err := entity.Activate("editor")
		require.NoError(t, err)
		assert.True(t, entity.IsActive())
	})
}

func TestReconstructCategory(t *testing.T) {
	id := uuid.New()
	code, _ := uomcategory.NewCode("VOLUME")
	createdAt := time.Now().Add(-24 * time.Hour)
	updatedAt := time.Now()
	updatedBy := "updater"

	entity := uomcategory.ReconstructCategory(
		id, code, "Volume", "Volume units", true,
		createdAt, "creator", &updatedAt, &updatedBy, nil, nil,
	)

	assert.Equal(t, id, entity.ID())
	assert.Equal(t, "VOLUME", entity.Code().String())
	assert.Equal(t, "Volume", entity.Name())
	assert.Equal(t, "Volume units", entity.Description())
	assert.True(t, entity.IsActive())
	assert.Equal(t, createdAt, entity.CreatedAt())
	assert.Equal(t, "creator", entity.CreatedBy())
	assert.Equal(t, updatedAt, *entity.UpdatedAt())
	assert.Equal(t, "updater", *entity.UpdatedBy())
	assert.False(t, entity.IsDeleted())
}
