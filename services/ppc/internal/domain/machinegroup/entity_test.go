package machinegroup_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/machinegroup"
)

func mustArea(t *testing.T, s string) area.Area {
	t.Helper()
	a, err := area.New(s)
	require.NoError(t, err)
	return a
}

func TestNewMachineGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		groupName string
		areaStr   string
		createdBy string
		wantErr   error
	}{
		{"valid", "Line A", "TXT", "tester", nil},
		{"empty name", "", "TXT", "tester", machinegroup.ErrEmptyName},
		{"name too long", strings.Repeat("x", 51), "TXT", "tester", machinegroup.ErrNameTooLong},
		{"empty created_by", "Line A", "SPG", "", machinegroup.ErrEmptyCreatedBy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity, err := machinegroup.NewMachineGroup(tt.groupName, mustArea(t, tt.areaStr), tt.createdBy)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, entity)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, entity)
			assert.Equal(t, tt.groupName, entity.Name())
			assert.Equal(t, tt.areaStr, entity.Area().String())
		})
	}
}

func TestNewMachineGroup_InvalidArea(t *testing.T) {
	t.Parallel()
	entity, err := machinegroup.NewMachineGroup("Line A", area.Area{}, "tester")
	assert.ErrorIs(t, err, machinegroup.ErrInvalidArea)
	assert.Nil(t, entity)
}

func TestMachineGroup_Update(t *testing.T) {
	t.Parallel()

	newName := "Line B"
	emptyName := ""
	spg := mustArea(t, "SPG")

	t.Run("updates name and area", func(t *testing.T) {
		t.Parallel()
		entity, err := machinegroup.NewMachineGroup("Line A", mustArea(t, "TXT"), "tester")
		require.NoError(t, err)
		require.NoError(t, entity.Update(&newName, &spg, "editor"))
		assert.Equal(t, newName, entity.Name())
		assert.Equal(t, "SPG", entity.Area().String())
		require.NotNil(t, entity.UpdatedBy())
		assert.Equal(t, "editor", *entity.UpdatedBy())
	})

	t.Run("rejects empty name", func(t *testing.T) {
		t.Parallel()
		entity, err := machinegroup.NewMachineGroup("Line A", mustArea(t, "TXT"), "tester")
		require.NoError(t, err)
		assert.ErrorIs(t, entity.Update(&emptyName, nil, "editor"), machinegroup.ErrEmptyName)
	})
}
