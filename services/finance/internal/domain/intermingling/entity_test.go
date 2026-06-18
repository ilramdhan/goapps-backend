package intermingling_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

func TestNew_Success(t *testing.T) {
	e, err := intermingling.New("INM001", "Intermingling A", 1500.0, "", "admin")
	require.NoError(t, err)
	assert.Equal(t, "INM001", e.Code())
	assert.Equal(t, "Intermingling A", e.Name())
	assert.Equal(t, 1500.0, e.CostPerKg())
	assert.True(t, e.IsActive())
}

func TestNew_EmptyCode(t *testing.T) {
	_, err := intermingling.New("", "Intermingling A", 0, "", "admin")
	assert.ErrorIs(t, err, intermingling.ErrEmptyCode)
}

func TestNew_EmptyName(t *testing.T) {
	_, err := intermingling.New("INM001", "", 0, "", "admin")
	assert.ErrorIs(t, err, intermingling.ErrEmptyName)
}

func TestNew_NegativeCost(t *testing.T) {
	_, err := intermingling.New("INM001", "Name", -1.0, "", "admin")
	assert.ErrorIs(t, err, intermingling.ErrInvalidCost)
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	e, err := intermingling.New("INM001", "Name", 0, "", "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), intermingling.ErrAlreadyDeleted)
}
