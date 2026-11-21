package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

type fakeMachineDefs struct {
	m   map[int64]map[string]float64
	err error
}

func (f fakeMachineDefs) LoadMachineParamDefaults(context.Context, []int64) (map[int64]map[string]float64, error) {
	return f.m, f.err
}

func TestApplyMachineDefaults(t *testing.T) {
	mk := func() []*financev1.RequiredParamEntry {
		return []*financev1.RequiredParamEntry{
			{ParamCode: "MB_COST_MKT", ValueNumeric: "0.1"},
			{ParamCode: "STEAM_COST_CNG"},
		}
	}
	h := (&CostProductParameterHandler{}).WithMachineParamDefaults(fakeMachineDefs{m: map[int64]map[string]float64{
		7: {"MB_COST_MKT": 0.08},
	}})

	es := mk()
	h.applyMachineDefaults(context.Background(), 7, es)
	assert.Equal(t, "0.08", es[0].MachineDefaultValue)
	assert.Equal(t, "0.1", es[0].ValueNumeric, "stored value untouched")
	assert.Empty(t, es[1].MachineDefaultValue, "param not configured for this machine")

	es = mk()
	h.applyMachineDefaults(context.Background(), 8, es) // product on another machine
	assert.Empty(t, es[0].MachineDefaultValue)

	es = mk()
	(&CostProductParameterHandler{}).applyMachineDefaults(context.Background(), 7, es) // no loader
	assert.Empty(t, es[0].MachineDefaultValue)

	es = mk()
	(&CostProductParameterHandler{}).WithMachineParamDefaults(fakeMachineDefs{err: errors.New("db")}).
		applyMachineDefaults(context.Background(), 7, es)
	assert.Empty(t, es[0].MachineDefaultValue, "loader failure leaves the form as before")
}
