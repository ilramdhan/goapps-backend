// Package parameter provides domain layer tests for Parameter entity.
package parameter_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

func TestNewCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType error
	}{
		{name: "valid code - simple", input: "SPEED", wantErr: false},
		{name: "valid code - with underscore", input: "ELEC_RATE", wantErr: false},
		{name: "valid code - with numbers", input: "PARAM123", wantErr: false},
		{name: "invalid - empty", input: "", wantErr: true, errType: parameter.ErrEmptyCode},
		{name: "invalid - lowercase", input: "speed", wantErr: true, errType: parameter.ErrInvalidCodeFormat},
		{name: "invalid - starts with number", input: "1SPEED", wantErr: true, errType: parameter.ErrInvalidCodeFormat},
		{name: "invalid - special characters", input: "SP@ED", wantErr: true, errType: parameter.ErrInvalidCodeFormat},
		{name: "invalid - too long", input: "ABCDEFGHIJKLMNOPQRSTUVWXYZ", wantErr: true, errType: parameter.ErrCodeTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := parameter.NewCode(tt.input)
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

func TestNewDataType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid - NUMBER", input: "NUMBER", wantErr: false},
		{name: "valid - TEXT", input: "TEXT", wantErr: false},
		{name: "valid - BOOLEAN", input: "BOOLEAN", wantErr: false},
		{name: "valid - lowercase (auto uppercase)", input: "number", wantErr: false},
		{name: "invalid - empty", input: "", wantErr: true},
		{name: "invalid - unknown", input: "DECIMAL", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt, err := parameter.NewDataType(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, dt.IsValid())
			}
		})
	}
}

func TestNewParamCategory(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid - INPUT", input: "INPUT", wantErr: false},
		{name: "valid - RATE", input: "RATE", wantErr: false},
		{name: "valid - CALCULATED", input: "CALCULATED", wantErr: false},
		{name: "valid - lowercase (auto uppercase)", input: "input", wantErr: false},
		{name: "invalid - empty", input: "", wantErr: true},
		{name: "invalid - unknown", input: "OUTPUT", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, err := parameter.NewParamCategory(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, cat.IsValid())
			}
		})
	}
}

func TestNewParameter(t *testing.T) {
	t.Run("valid parameter creation", func(t *testing.T) {
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")
		defVal := "100.5"
		minVal := "0"
		maxVal := "9999"

		entity, err := parameter.NewParameter(code, "Speed", "Spd", dt, cat, nil, &defVal, &minVal, &maxVal, "admin")

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, entity.ID())
		assert.Equal(t, "SPEED", entity.Code().String())
		assert.Equal(t, "Speed", entity.Name())
		assert.Equal(t, "Spd", entity.ShortName())
		assert.Equal(t, "NUMBER", entity.DataType().String())
		assert.Equal(t, "INPUT", entity.ParamCategory().String())
		assert.Nil(t, entity.UOMID())
		assert.Equal(t, "100.5", *entity.DefaultValue())
		assert.Equal(t, "0", *entity.MinValue())
		assert.Equal(t, "9999", *entity.MaxValue())
		assert.True(t, entity.IsActive())
		assert.Equal(t, "admin", entity.CreatedBy())
		assert.False(t, entity.CreatedAt().IsZero())
	})

	t.Run("valid parameter with UOM reference", func(t *testing.T) {
		code, _ := parameter.NewCode("DENIER")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")
		uomID := uuid.New()

		entity, err := parameter.NewParameter(code, "Denier", "", dt, cat, &uomID, nil, nil, nil, "admin")

		require.NoError(t, err)
		assert.NotNil(t, entity.UOMID())
		assert.Equal(t, uomID, *entity.UOMID())
	})

	t.Run("invalid - empty name", func(t *testing.T) {
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")

		_, err := parameter.NewParameter(code, "", "", dt, cat, nil, nil, nil, nil, "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrEmptyName)
	})

	t.Run("invalid - name too long", func(t *testing.T) {
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")
		longName := string(make([]byte, 201))

		_, err := parameter.NewParameter(code, longName, "", dt, cat, nil, nil, nil, nil, "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrNameTooLong)
	})

	t.Run("invalid - short name too long", func(t *testing.T) {
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")
		longShort := string(make([]byte, 51))

		_, err := parameter.NewParameter(code, "Speed", longShort, dt, cat, nil, nil, nil, nil, "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrShortNameTooLong)
	})

	t.Run("invalid - empty created by", func(t *testing.T) {
		code, _ := parameter.NewCode("SPEED")
		dt, _ := parameter.NewDataType("NUMBER")
		cat, _ := parameter.NewParamCategory("INPUT")

		_, err := parameter.NewParameter(code, "Speed", "", dt, cat, nil, nil, nil, nil, "")

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrEmptyCreatedBy)
	})
}

func TestParameter_Update(t *testing.T) {
	code, _ := parameter.NewCode("SPEED")
	dt, _ := parameter.NewDataType("NUMBER")
	cat, _ := parameter.NewParamCategory("INPUT")
	entity, _ := parameter.NewParameter(code, "Speed", "Spd", dt, cat, nil, nil, nil, nil, "admin")

	t.Run("update name", func(t *testing.T) {
		newName := "Speed Updated"
		err := entity.Update(&newName, nil, nil, nil, nil, nil, nil, nil, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "Speed Updated", entity.Name())
		assert.NotNil(t, entity.UpdatedBy())
		assert.Equal(t, "editor", *entity.UpdatedBy())
	})

	t.Run("update data type", func(t *testing.T) {
		newDT, _ := parameter.NewDataType("TEXT")
		err := entity.Update(nil, nil, &newDT, nil, nil, nil, nil, nil, nil, "editor2")

		require.NoError(t, err)
		assert.Equal(t, "TEXT", entity.DataType().String())
	})

	t.Run("update category", func(t *testing.T) {
		newCat, _ := parameter.NewParamCategory("RATE")
		err := entity.Update(nil, nil, nil, &newCat, nil, nil, nil, nil, nil, "editor3")

		require.NoError(t, err)
		assert.Equal(t, "RATE", entity.ParamCategory().String())
	})

	t.Run("update uom reference", func(t *testing.T) {
		uomID := uuid.New()
		uomIDPtr := &uomID
		err := entity.Update(nil, nil, nil, nil, &uomIDPtr, nil, nil, nil, nil, "editor4")

		require.NoError(t, err)
		assert.NotNil(t, entity.UOMID())
		assert.Equal(t, uomID, *entity.UOMID())
	})

	t.Run("clear uom reference", func(t *testing.T) {
		var nilUOM *uuid.UUID
		err := entity.Update(nil, nil, nil, nil, &nilUOM, nil, nil, nil, nil, "editor5")

		require.NoError(t, err)
		assert.Nil(t, entity.UOMID())
	})

	t.Run("update is_active", func(t *testing.T) {
		inactive := false
		err := entity.Update(nil, nil, nil, nil, nil, nil, nil, nil, &inactive, "editor6")

		require.NoError(t, err)
		assert.False(t, entity.IsActive())
	})

	t.Run("update default value", func(t *testing.T) {
		newDefault := "200.5"
		newDefaultPtr := &newDefault
		err := entity.Update(nil, nil, nil, nil, nil, &newDefaultPtr, nil, nil, nil, "editor7")

		require.NoError(t, err)
		assert.Equal(t, "200.5", *entity.DefaultValue())
	})
}

func TestReconstructParameter(t *testing.T) {
	id := uuid.New()
	code, _ := parameter.NewCode("ELEC_RATE")
	dt, _ := parameter.NewDataType("NUMBER")
	cat, _ := parameter.NewParamCategory("RATE")
	uomID := uuid.New()
	defVal := "0.5"
	minVal := "0"
	maxVal := "100"
	createdAt := time.Now().Add(-24 * time.Hour)
	updatedAt := time.Now()
	updatedBy := "updater"

	entity := parameter.ReconstructParameter(
		id, code, "Electricity Rate", "Elec", dt, cat,
		&uomID, "KWH", "Kilowatt Hour",
		&defVal, &minVal, &maxVal,
		true, createdAt, "creator",
		&updatedAt, &updatedBy, nil, nil,
	)

	assert.Equal(t, id, entity.ID())
	assert.Equal(t, "ELEC_RATE", entity.Code().String())
	assert.Equal(t, "Electricity Rate", entity.Name())
	assert.Equal(t, "Elec", entity.ShortName())
	assert.Equal(t, "NUMBER", entity.DataType().String())
	assert.Equal(t, "RATE", entity.ParamCategory().String())
	assert.Equal(t, uomID, *entity.UOMID())
	assert.Equal(t, "KWH", entity.UOMCode())
	assert.Equal(t, "Kilowatt Hour", entity.UOMName())
	assert.Equal(t, "0.5", *entity.DefaultValue())
	assert.Equal(t, "0", *entity.MinValue())
	assert.Equal(t, "100", *entity.MaxValue())
	assert.True(t, entity.IsActive())
	assert.Equal(t, createdAt, entity.CreatedAt())
	assert.Equal(t, "creator", entity.CreatedBy())
	assert.NotNil(t, entity.UpdatedAt())
	assert.Equal(t, "updater", *entity.UpdatedBy())
	assert.False(t, entity.IsDeleted())
}

func TestParameter_SoftDelete(t *testing.T) {
	code, _ := parameter.NewCode("SPEED")
	dt, _ := parameter.NewDataType("NUMBER")
	cat, _ := parameter.NewParamCategory("INPUT")
	entity, _ := parameter.NewParameter(code, "Speed", "", dt, cat, nil, nil, nil, nil, "admin")

	t.Run("success", func(t *testing.T) {
		err := entity.SoftDelete("deleter")

		require.NoError(t, err)
		assert.True(t, entity.IsDeleted())
		assert.False(t, entity.IsActive())
		assert.NotNil(t, entity.DeletedAt())
		assert.Equal(t, "deleter", *entity.DeletedBy())
	})

	t.Run("error - already deleted", func(t *testing.T) {
		err := entity.SoftDelete("deleter2")

		assert.Error(t, err)
		assert.ErrorIs(t, err, parameter.ErrAlreadyDeleted)
	})
}

func TestDataType_IsValid(t *testing.T) {
	validTypes := []string{"NUMBER", "TEXT", "BOOLEAN"}
	for _, dtStr := range validTypes {
		dt, err := parameter.NewDataType(dtStr)
		assert.NoError(t, err)
		assert.True(t, dt.IsValid())
	}
}

func TestParamCategory_IsValid(t *testing.T) {
	validCategories := []string{"INPUT", "RATE", "CALCULATED"}
	for _, catStr := range validCategories {
		cat, err := parameter.NewParamCategory(catStr)
		assert.NoError(t, err)
		assert.True(t, cat.IsValid())
	}
}
