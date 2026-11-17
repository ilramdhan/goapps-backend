package lookupsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// Regression: MC_SPEED / MC_EFFICIENCY are per-product parameters now. The machine
// master must never act as a fill source for them.

func TestLookupReaderColumns_MachineExcludesSpeedEfficiency(t *testing.T) {
	cols := LookupReaderColumns()[MasterMachine]
	require.NotNil(t, cols)
	assert.NotContains(t, cols, "mc_speed")
	assert.NotContains(t, cols, "mc_efficiency")
	for _, want := range []string{
		"no_of_position", "no_of_end", "machine_rpm", "power_per_day", "mp_per_day",
		"ohs_per_day", "spares_per_day", "kgs_lost_change",
		"vb1_qty", "vb2_qty", "vb3_qty", "vb4_qty", "vb5_qty", "mc_weightage",
	} {
		assert.Contains(t, cols, want)
	}
}

type fakeMachineRepo struct {
	machine.Repository
	e *machine.Entity
}

func (f fakeMachineRepo) GetByCode(_ context.Context, _ string) (*machine.Entity, error) {
	return f.e, nil
}

type fakeFillParamRepo struct {
	parameter.Repository
	kids []*parameter.Parameter
}

func (f fakeFillParamRepo) GetByFillGroup(_ context.Context, _ string) ([]*parameter.Parameter, error) {
	return f.kids, nil
}

func newSourceParam(t *testing.T, code, srcCol string) *parameter.Parameter {
	t.Helper()
	c, err := parameter.NewCode(code)
	require.NoError(t, err)
	p, err := parameter.NewParameter(c, code+" name", "", parameter.DataTypeNumber, parameter.ParamCategoryInput, nil, nil, nil, nil,
		parameter.CostingMetadata{LookupFillGroupCode: "MC_NAME", LookupSourceColumn: srcCol}, "test")
	require.NoError(t, err)
	return p
}

func TestFillFromMachine_SkipsMisconfiguredSpeedEfficiency(t *testing.T) {
	mc, err := machine.New("M1", "Machine One", "DTY", "A", 96, 2,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", "admin")
	require.NoError(t, err)

	r := NewResolver(fakeMachineRepo{e: mc}, nil, nil, nil, nil, nil, fakeFillParamRepo{kids: []*parameter.Parameter{
		newSourceParam(t, "MC_SPEED", "mc_speed"),
		newSourceParam(t, "MC_EFFICIENCY", "mc_efficiency"),
		newSourceParam(t, "NO_OF_POSITION", "no_of_position"),
	}})

	res, err := r.Resolve(context.Background(), MasterMachine, "M1", "MC_NAME")
	require.NoError(t, err)
	assert.NotContains(t, res.Numeric, "MC_SPEED")
	assert.NotContains(t, res.Numeric, "MC_EFFICIENCY")
	assert.Equal(t, 96.0, res.Numeric["NO_OF_POSITION"])
	assert.Equal(t, "Machine One (DTY) — 96 pos", res.DisplayLabel)
}
