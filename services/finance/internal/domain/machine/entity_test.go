package machine_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

func TestNew_Success(t *testing.T) {
	e, err := machine.New("BT-D", "Barmag DTY", "DTY", "Plant A", 504, 1, 800.0, nil, 92.0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	require.NoError(t, err)
	assert.Equal(t, "BT-D", e.Code())
	assert.Equal(t, "Barmag DTY", e.Name())
	assert.Equal(t, "DTY", e.MCType())
	assert.Equal(t, 504, e.NoOfPosition())
	assert.Equal(t, 92.0, e.MCEfficiency())
	assert.True(t, e.IsActive())
	assert.False(t, e.IsDeleted())
}

func TestNew_EmptyCode(t *testing.T) {
	_, err := machine.New("", "Barmag DTY", "DTY", "", 0, 1, 0, nil, 95, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	assert.ErrorIs(t, err, machine.ErrEmptyCode)
}

func TestNew_EmptyName(t *testing.T) {
	_, err := machine.New("BT-D", "", "DTY", "", 0, 1, 0, nil, 95, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	assert.ErrorIs(t, err, machine.ErrEmptyName)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	_, err := machine.New("BT-D", "Barmag DTY", "", "", 0, 1, 0, nil, 95, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "")
	assert.ErrorIs(t, err, machine.ErrEmptyCreatedBy)
}

func TestUpdate_Success(t *testing.T) {
	e, err := machine.New("BT-D", "Old Name", "", "", 0, 1, 0, nil, 95, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	require.NoError(t, err)
	newName := "New Name"
	err = e.Update(machine.UpdateInput{Name: &newName}, "editor")
	require.NoError(t, err)
	assert.Equal(t, "New Name", e.Name())
	assert.NotNil(t, e.UpdatedAt())
}

func TestSoftDelete_Success(t *testing.T) {
	e, err := machine.New("BT-D", "Machine", "", "", 0, 1, 0, nil, 95, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.True(t, e.IsDeleted())
	assert.False(t, e.IsActive())
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	e, err := machine.New("BT-D", "Machine", "", "", 0, 1, 0, nil, 95, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), machine.ErrAlreadyDeleted)
}
