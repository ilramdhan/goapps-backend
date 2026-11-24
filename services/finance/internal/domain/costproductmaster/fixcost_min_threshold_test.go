package costproductmaster_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

func TestSetFixCostMinThreshold(t *testing.T) {
	at := time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC)
	p := rmModeProduct(true)
	require.Nil(t, p.FixCostMinThreshold())

	changed, err := p.SetFixCostMinThreshold(0.05, "u1", at)
	require.NoError(t, err)
	assert.True(t, changed)
	require.NotNil(t, p.FixCostMinThreshold())
	assert.InDelta(t, 0.05, *p.FixCostMinThreshold(), 1e-12)
	assert.Equal(t, "u1", p.UpdatedBy())
	assert.Equal(t, at, p.UpdatedAt())

	changed, err = p.SetFixCostMinThreshold(0.05, "u2", at.Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, changed, "same value is a no-op")
	assert.Equal(t, "u1", p.UpdatedBy())

	changed, err = p.SetFixCostMinThreshold(0, "u3", at)
	require.NoError(t, err)
	assert.True(t, changed, "0 = off is stored, not NULL")
	require.NotNil(t, p.FixCostMinThreshold())
	assert.Zero(t, *p.FixCostMinThreshold())

	// unset (auto) -> 0 is a change too
	q := rmModeProduct(true)
	changed, err = q.SetFixCostMinThreshold(0, "u", at)
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestSetFixCostMinThreshold_InvalidAndInactive(t *testing.T) {
	at := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	p := rmModeProduct(true)
	for _, v := range []float64{-0.01, 1.01} {
		_, err := p.SetFixCostMinThreshold(v, "u", at)
		require.ErrorIs(t, err, costproductmaster.ErrInvalidFixCostMinThreshold)
	}
	assert.Nil(t, p.FixCostMinThreshold())

	_, err := rmModeProduct(false).SetFixCostMinThreshold(0.1, "u", at)
	require.ErrorIs(t, err, costproductmaster.ErrInactive)
}
