package workorder_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

type fakeDefs struct{ defs []workorder.ParamDef }

func (f fakeDefs) ListParamDefs(context.Context, string) ([]workorder.ParamDef, error) {
	return f.defs, nil
}

type fakeVals struct {
	vals map[string]workorder.TypedValue
}

func (f fakeVals) ProductMachineValues(context.Context, int64, int64) (map[string]workorder.TypedValue, error) {
	return f.vals, nil
}
func (f fakeVals) ProductValues(context.Context, int64, []string) (map[string]workorder.TypedValue, error) {
	return f.vals, nil
}

type fakeRef struct {
	vals map[string]workorder.TypedValue
}

func (f fakeRef) RefParamValues(context.Context, int64) (map[string]workorder.TypedValue, error) {
	return f.vals, nil
}

func num(v float64) *float64 { return &v }

var testDefs = []workorder.ParamDef{
	{ParamID: "11111111-1111-1111-1111-111111111111", ParamCode: "SPEED", DataType: "NUMBER", DisplayGroup: "Machine", DefaultValue: "500"},
	{ParamID: "22222222-2222-2222-2222-222222222222", ParamCode: "DENIER", DataType: "NUMBER", DisplayGroup: "Machine", DefaultValue: "150"},
}

func TestResolve_LayerPriority_WORefWins(t *testing.T) {
	r := workorder.NewResolver(
		fakeDefs{testDefs},
		fakeVals{map[string]workorder.TypedValue{"11111111-1111-1111-1111-111111111111": {Num: num(700)}}},
		fakeVals{map[string]workorder.TypedValue{"11111111-1111-1111-1111-111111111111": {Num: num(600)}}},
		fakeRef{map[string]workorder.TypedValue{"11111111-1111-1111-1111-111111111111": {Num: num(760)}}},
	)
	ref := int64(5)
	out, err := r.Resolve(context.Background(), workorder.ResolveRequest{ProductSysID: 1, MachineID: 2, RefWoID: &ref, DisplayGroup: "Machine"})
	require.NoError(t, err)
	require.Len(t, out, 2)
	speed := out[0]
	assert.Equal(t, "SPEED", speed.ParamCode)
	assert.Equal(t, 760.0, *speed.Num)
	assert.Equal(t, workorder.SourceWORef, speed.Source)
	assert.True(t, speed.IsDual) // SPEED is a dual param
}

func TestResolve_FallsThroughLayers(t *testing.T) {
	// No WO-ref; product_machine has SPEED; DENIER falls to default.
	r := workorder.NewResolver(
		fakeDefs{testDefs},
		fakeVals{map[string]workorder.TypedValue{"11111111-1111-1111-1111-111111111111": {Num: num(720)}}},
		fakeVals{nil},
		nil,
	)
	out, err := r.Resolve(context.Background(), workorder.ResolveRequest{ProductSysID: 1, MachineID: 2, DisplayGroup: "Machine"})
	require.NoError(t, err)
	require.Len(t, out, 2)

	assert.Equal(t, 720.0, *out[0].Num)
	assert.Equal(t, workorder.SourceProductMachine, out[0].Source)

	assert.Equal(t, "DENIER", out[1].ParamCode)
	assert.Equal(t, 150.0, *out[1].Num) // from default_value
	assert.Equal(t, workorder.SourceDefault, out[1].Source)
	assert.False(t, out[1].IsDual)
}

func TestResolve_ProductLayer(t *testing.T) {
	r := workorder.NewResolver(
		fakeDefs{testDefs},
		fakeVals{nil},
		fakeVals{map[string]workorder.TypedValue{"22222222-2222-2222-2222-222222222222": {Num: num(300)}}},
		nil,
	)
	out, err := r.Resolve(context.Background(), workorder.ResolveRequest{ProductSysID: 1, MachineID: 2, DisplayGroup: "Machine"})
	require.NoError(t, err)
	assert.Equal(t, 300.0, *out[1].Num)
	assert.Equal(t, workorder.SourceProduct, out[1].Source)
}

func TestPinWellKnown(t *testing.T) {
	pinned := workorder.PinWellKnown(testDefs)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", pinned[workorder.WellKnownDenier])
	_, hasSpeed := pinned["SPEED"]
	assert.False(t, hasSpeed) // SPEED is not a well-known efficiency param
}

func TestIsDualCode(t *testing.T) {
	assert.True(t, workorder.IsDualCode("SPEED"))
	assert.True(t, workorder.IsDualCode("OPU"))
	assert.False(t, workorder.IsDualCode("DENIER"))
}
