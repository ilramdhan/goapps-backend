package boxbobbincost_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

func TestNew_Success(t *testing.T) {
	e, err := boxbobbincost.New("BBC001", "Box Bobbin A", "CAPTIVE", 24, "", nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	assert.Equal(t, "BBC001", e.Code())
	assert.Equal(t, "Box Bobbin A", e.Name())
	assert.Equal(t, "CAPTIVE", e.BBCType())
	assert.Equal(t, 24, e.NoOfBob())
	assert.True(t, e.IsActive())
}

func TestNew_EmptyCode(t *testing.T) {
	_, err := boxbobbincost.New("", "Box Bobbin A", "CAPTIVE", 24, "", nil, nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, boxbobbincost.ErrEmptyCode)
}

func TestNew_EmptyName(t *testing.T) {
	_, err := boxbobbincost.New("BBC001", "", "CAPTIVE", 24, "", nil, nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, boxbobbincost.ErrEmptyName)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	_, err := boxbobbincost.New("BBC001", "Box Bobbin A", "CAPTIVE", 24, "", nil, nil, nil, nil, nil, nil, "")
	assert.ErrorIs(t, err, boxbobbincost.ErrEmptyCreatedBy)
}

func TestSoftDelete_Success(t *testing.T) {
	e, err := boxbobbincost.New("BBC001", "Box Bobbin A", "CAPTIVE", 24, "", nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.True(t, e.IsDeleted())
	assert.False(t, e.IsActive())
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	e, err := boxbobbincost.New("BBC001", "Box Bobbin A", "CAPTIVE", 24, "", nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), boxbobbincost.ErrAlreadyDeleted)
}
