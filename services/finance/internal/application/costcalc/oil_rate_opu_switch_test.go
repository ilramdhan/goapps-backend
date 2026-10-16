package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// =============================================================================
// OPU is the oil switch (finance rule, follow-up OIL ACY/ATY): an oil-class
// product with OPU absent or <= 0 has no oil, so OIL_RATE is not resolved or
// validated and can never block the product. OPU > 0 keeps the old behavior.
// =============================================================================

// disallowedPTY is a PTY product whose stored OIL_NAME is not in the allowed
// set — blocked today when OPU > 0.
func disallowedPTY() *OilInput { return ptyOil(oilGroupSpin) }

func TestComputeProduct_OilOPUSwitch(t *testing.T) {
	t.Parallel()

	coningRow := map[string]RMCostRates{oilGroupConing + "|": {CrRate: 0, SrRate: 2.2869, PrRate: 3}}
	spinRow := map[string]RMCostRates{oilGroupSpin + "|": {CrRate: 1.5}}

	tests := []struct {
		name        string
		c           oilCase
		dropOPU     bool // remove OPU from CAPP entirely
		wantBlocked string
		wantOilRate float64
		wantOilCost float64
		wantOilGain float64
		wantFlags   [3]float64
		wantNote    bool
	}{
		{
			name:        "OPU 0 + disallowed group: not blocked, OIL_RATE 0, note",
			c:           oilCase{oil: disallowedPTY(), opu: 0, wastePerc: 0.7, cappOilRate: floatPtr(9.99), rmCosts: spinRow},
			wantOilRate: 0, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{1, 0, 0}, wantNote: true,
		},
		{
			name:        "OPU 0 + missing rate row: not blocked",
			c:           oilCase{oil: ptyOil(oilGroupConing), opu: 0, wastePerc: 0.7},
			wantOilRate: 0, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{1, 0, 0}, wantNote: true,
		},
		{
			name:        "OPU negative + no group configured: not blocked",
			c:           oilCase{oil: &OilInput{Class: OilClassPTY, TypeCode: "ATY"}, opu: -1, wastePerc: 0.7},
			wantOilRate: 0, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{1, 0, 0}, wantNote: true,
		},
		{
			name:        "OPU absent + disallowed group: not blocked",
			c:           oilCase{oil: disallowedPTY(), wastePerc: 0.7, rmCosts: spinRow},
			dropOPU:     true,
			wantOilRate: 0, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{1, 0, 0}, wantNote: true,
		},
		{
			name: "OPU 0 + all-zero rates: not blocked",
			c: oilCase{
				oil: ptyOil(oilGroupConing), opu: 0, wastePerc: 0.7,
				rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CostVal: 7}},
			},
			wantOilRate: 0, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{1, 0, 0}, wantNote: true,
		},
		{
			name:        "OPU > 0 + disallowed group: still blocked",
			c:           oilCase{oil: disallowedPTY(), opu: 2.2, wastePerc: 0.7, rmCosts: spinRow},
			wantBlocked: "oil group " + oilGroupSpin + " not allowed for type PTY",
		},
		{
			name:        "OPU > 0 valid: same rate as before",
			c:           oilCase{oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7, rmCosts: coningRow},
			wantOilRate: 2.2869, wantOilCost: 0.0569286126480872, wantOilGain: -0.0503118,
			wantFlags: [3]float64{1, 0, 0},
		},
		{
			name:        "no oil class + OPU 0: CAPP OIL_RATE kept, no note",
			c:           oilCase{oil: nil, opu: 0, wastePerc: 0.7, cappOilRate: floatPtr(5)},
			wantOilRate: 5, wantOilCost: 0, wantOilGain: 0,
			wantFlags: [3]float64{0, 0, 0},
		},
		{
			name:        "POY OPU > 0: gain default branch unaffected",
			c:           oilCase{oil: poyOil(), opu: 0.6, wastePerc: 0.7, rmCosts: spinRow},
			wantOilRate: 1.5, wantOilCost: 0.010183645066024, wantOilGain: -0.002,
			wantFlags: [3]float64{0, 1, 0},
		},
		{
			name:        "POY OPU 0: rate skipped, gain default branch still applies",
			c:           oilCase{oil: poyOil(), opu: 0, wastePerc: 0.7},
			wantOilRate: 0, wantOilCost: 0, wantOilGain: -0.002,
			wantFlags: [3]float64{0, 1, 0}, wantNote: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := oilInput(tt.c)
			if tt.dropOPU {
				delete(in.CAPP, ScopeKeyOPU)
			}
			out, err := ComputeProduct(context.Background(), in)
			if tt.wantBlocked != "" {
				require.Error(t, err)
				assert.Nil(t, out)
				assert.ErrorIs(t, err, costcalcdom.ErrMissingRMCost)
				assert.Contains(t, err.Error(), tt.wantBlocked)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, out)
			snap := out.ParamSnapshot

			rate, ok := snap[ScopeKeyOilRate]
			require.True(t, ok, "OIL_RATE must be in the snapshot")
			assert.InDelta(t, tt.wantOilRate, rate, oilTol, "OIL_RATE")
			assert.InDelta(t, tt.wantOilCost, snap["OIL_COST"], oilTol, "OIL_COST")
			assert.InDelta(t, tt.wantOilGain, snap["OIL_GAIN"], oilTol, "OIL_GAIN")
			for i, key := range []string{ScopeKeyIsPTY, ScopeKeyIsPOY, ScopeKeyIsSuperba} {
				assert.InDelta(t, tt.wantFlags[i], snap[key], 0, key)
			}

			if tt.wantNote {
				require.Len(t, out.Notes, 1)
				assert.Contains(t, out.Notes[0], "OPU is 0 or missing")
			} else {
				assert.Nil(t, out.Notes)
			}
		})
	}
}

func TestAppendNote_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	base := make([]string, 1, 4)
	base[0] = "a"
	got := appendNote(base, "b")
	assert.Equal(t, []string{"a", "b"}, got)
	assert.Equal(t, []string{"a"}, base)
	assert.Equal(t, "", base[:2][1], "backing array untouched")
	assert.Equal(t, base, appendNote(base, ""))
	assert.Nil(t, appendNote(nil, ""))
}
