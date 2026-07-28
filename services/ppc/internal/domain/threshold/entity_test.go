package threshold_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/threshold"
)

func TestNewConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   string
		unit    string
		warning float64
		block   float64
		wantErr error
	}{
		{"valid pct", "SYSTEM", "PCT", 3, 6, nil},
		{"valid doff", "MACHINE_GROUP", "DOFF", 600, 1200, nil},
		{"invalid level", "BOGUS", "PCT", 3, 6, threshold.ErrInvalidLevel},
		{"invalid unit", "SYSTEM", "KG", 3, 6, threshold.ErrInvalidUnit},
		{"block below warning", "SYSTEM", "PCT", 6, 3, threshold.ErrInvalidThresholds},
		{"block equals warning ok", "PRODUCT", "PCT", 5, 5, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity, err := threshold.NewConfig(tt.level, nil, tt.unit, tt.warning, tt.block, "", "tester")
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, entity)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, entity)
			assert.Equal(t, tt.level, entity.Level())
			assert.Equal(t, tt.unit, entity.Unit())
		})
	}
}

func TestNewConfig_EmptyCreatedBy(t *testing.T) {
	t.Parallel()
	entity, err := threshold.NewConfig("SYSTEM", nil, "PCT", 3, 6, "", "")
	assert.ErrorIs(t, err, threshold.ErrEmptyCreatedBy)
	assert.Nil(t, entity)
}

func TestConfig_Update_RejectsInvertedThresholds(t *testing.T) {
	t.Parallel()
	entity, err := threshold.NewConfig("SYSTEM", nil, "PCT", 3, 6, "", "tester")
	require.NoError(t, err)

	warn := 9.0
	assert.ErrorIs(t, entity.Update(nil, &warn, nil, nil, nil, "editor"), threshold.ErrInvalidThresholds)
}

func TestIsValidLevelAndUnit(t *testing.T) {
	t.Parallel()
	assert.True(t, threshold.IsValidLevel("WO"))
	assert.False(t, threshold.IsValidLevel("wo"))
	assert.True(t, threshold.IsValidUnit("DOFF"))
	assert.False(t, threshold.IsValidUnit(""))
}
