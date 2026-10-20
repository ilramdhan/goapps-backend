package costproductmaster_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

func rmModeProduct(active bool) *costproductmaster.CostProductMaster {
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	return costproductmaster.Reconstruct(1, "P1", 3, "Product", "", "AX", "", "", "", "", nil, "",
		active, at, "seed", at, "seed", "", "", "", "", "", false)
}

func TestNormalizeRMRatioModeOverride(t *testing.T) {
	for in, want := range map[string]string{"": "", "  ": "", "INPUT": "INPUT", " denier ": "DENIER", "Input": "INPUT"} {
		got, err := costproductmaster.NormalizeRMRatioModeOverride(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"X", "DENIERS", "IN PUT"} {
		_, err := costproductmaster.NormalizeRMRatioModeOverride(in)
		assert.ErrorIs(t, err, costproductmaster.ErrInvalidRMRatioModeOverride, in)
	}
}

func TestSetRMRatioModeOverride(t *testing.T) {
	at := time.Date(2026, 10, 21, 1, 2, 3, 0, time.UTC)
	p := rmModeProduct(true)
	p.RestoreRMRatioMode("", "INPUT")

	changed, err := p.SetRMRatioModeOverride("denier", "u1", at)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "DENIER", p.RMRatioModeOverride())
	assert.Equal(t, "DENIER", p.EffectiveRMRatioMode())
	assert.Equal(t, "u1", p.UpdatedBy())
	assert.Equal(t, at, p.UpdatedAt())

	changed, err = p.SetRMRatioModeOverride("DENIER", "u2", at.Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, changed, "same value is a no-op")
	assert.Equal(t, "u1", p.UpdatedBy())

	changed, err = p.SetRMRatioModeOverride("", "u3", at)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Empty(t, p.RMRatioModeOverride())

	_, err = p.SetRMRatioModeOverride("X", "u4", at)
	require.ErrorIs(t, err, costproductmaster.ErrInvalidRMRatioModeOverride)
	assert.Empty(t, p.RMRatioModeOverride(), "invalid value leaves the override unchanged")

	_, err = rmModeProduct(false).SetRMRatioModeOverride("INPUT", "u", at)
	require.ErrorIs(t, err, costproductmaster.ErrInactive)
}
