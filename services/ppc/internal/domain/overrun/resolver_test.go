package overrun_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/overrun"
)

// fakeLookup returns a configured threshold per (level, ref) pair.
type fakeLookup struct {
	byLevel map[string]*overrun.Threshold
}

func (f *fakeLookup) FindThreshold(_ context.Context, level string, _ *int64) (*overrun.Threshold, error) {
	if th, ok := f.byLevel[level]; ok {
		return th, nil
	}
	return nil, nil
}

func TestResolve_MostSpecificWins(t *testing.T) {
	lookup := &fakeLookup{byLevel: map[string]*overrun.Threshold{
		overrun.LevelSystem:       {Level: overrun.LevelSystem, Unit: overrun.UnitPct, WarningValue: 3, BlockValue: 6},
		overrun.LevelMachineGroup: {Level: overrun.LevelMachineGroup, Unit: overrun.UnitPct, WarningValue: 4, BlockValue: 8},
		overrun.LevelProduct:      {Level: overrun.LevelProduct, Unit: overrun.UnitPct, WarningValue: 5, BlockValue: 10},
	}}
	r := overrun.NewResolver(lookup)

	th, err := r.Resolve(context.Background(), overrun.Scope{
		ProductSysID:   100,
		MachineGroupID: 3,
	})
	require.NoError(t, err)
	require.NotNil(t, th)
	assert.Equal(t, overrun.LevelProduct, th.Level) // PRODUCT beats MACHINE_GROUP + SYSTEM
}

func TestResolve_FallsBackToSystem(t *testing.T) {
	lookup := &fakeLookup{byLevel: map[string]*overrun.Threshold{
		overrun.LevelSystem: {Level: overrun.LevelSystem, Unit: overrun.UnitPct, WarningValue: 3, BlockValue: 6},
	}}
	r := overrun.NewResolver(lookup)

	th, err := r.Resolve(context.Background(), overrun.Scope{ProductSysID: 100, MachineGroupID: 3})
	require.NoError(t, err)
	require.NotNil(t, th)
	assert.Equal(t, overrun.LevelSystem, th.Level)
}

func TestResolve_MachineGroupBeatsSystem(t *testing.T) {
	lookup := &fakeLookup{byLevel: map[string]*overrun.Threshold{
		overrun.LevelSystem:       {Level: overrun.LevelSystem, Unit: overrun.UnitPct, WarningValue: 3, BlockValue: 6},
		overrun.LevelMachineGroup: {Level: overrun.LevelMachineGroup, Unit: overrun.UnitPct, WarningValue: 4, BlockValue: 8},
	}}
	r := overrun.NewResolver(lookup)

	th, err := r.Resolve(context.Background(), overrun.Scope{MachineGroupID: 3})
	require.NoError(t, err)
	assert.Equal(t, overrun.LevelMachineGroup, th.Level)
}

func TestEvaluate_PctBands(t *testing.T) {
	th := &overrun.Threshold{Unit: overrun.UnitPct, WarningValue: 3, BlockValue: 6}
	assert.Equal(t, overrun.StatusOK, overrun.Evaluate(th, 1000, 1020))      // +2% ok
	assert.Equal(t, overrun.StatusWarning, overrun.Evaluate(th, 1000, 1040)) // +4% warn
	assert.Equal(t, overrun.StatusBlocked, overrun.Evaluate(th, 1000, 1070)) // +7% block
}

func TestEvaluate_DoffAbsolute(t *testing.T) {
	th := &overrun.Threshold{Unit: overrun.UnitDoff, WarningValue: 600, BlockValue: 1200}
	assert.Equal(t, overrun.StatusOK, overrun.Evaluate(th, 1000, 1500))      // +500 ok
	assert.Equal(t, overrun.StatusWarning, overrun.Evaluate(th, 1000, 1700)) // +700 warn
	assert.Equal(t, overrun.StatusBlocked, overrun.Evaluate(th, 1000, 2300)) // +1300 block
}

func TestEvaluate_NilThresholdIsOK(t *testing.T) {
	assert.Equal(t, overrun.StatusOK, overrun.Evaluate(nil, 1000, 5000))
}
