package mbspin_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func f(v float64) *float64 { return &v }

// mb-comparison-p2 item 2: MB_SP_DOZING reader = effective LDR incl. manual adjustment.
func TestNumericFillReaders_RunLdrPct_EffectiveLDR(t *testing.T) {
	cases := []struct {
		name          string
		run, calc, ad *float64
		want          float64
		ok            bool
	}{
		{"adj nil -> raw run_ldr", f(1.60), f(1.50), nil, 1.60, true},
		{"adj zero -> raw run_ldr", f(1.60), f(1.50), f(0), 1.60, true},
		{"negative adj with calc", f(1.60), f(1.60), f(-0.15), 1.45, true},
		{"negative adj calc nil -> run_ldr base", f(1.60), nil, f(-0.15), 1.45, true},
		{"effective zero -> not filled", f(1.60), f(0.15), f(-0.15), 0, false},
		{"effective negative -> not filled", f(1.60), f(0.10), f(-0.15), 0, false},
		{"adj but no base at all", nil, nil, f(-0.15), 0, false},
		{"all nil", nil, nil, nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := mbspin.Reconstruct(uuid.New(), nil, nil, uuid.New(), "x", nil, nil, nil, nil, nil, nil, nil, nil, c.run, nil, nil, nil, true, time.Now(), "t", nil, nil, nil, nil)
			e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{LDRType: mbspin.LDRTypeActual, LDRCalculatedPct: c.calc, LDRAdjustmentPct: c.ad})
			got, ok := mbspin.NumericFillReaders["mbs_run_ldr_pct"](e)
			assert.Equal(t, c.ok, ok)
			assert.InDelta(t, c.want, got, 1e-9)
		})
	}
}

func spinWithAdj(calc, adj float64) *mbspin.Entity {
	e := mbspin.Reconstruct(uuid.New(), nil, nil, uuid.New(), "x", nil, nil, nil, nil, nil, nil, nil, nil, f(calc), nil, nil, nil, true, time.Now(), "t", nil, nil, nil, nil)
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{LDRType: mbspin.LDRTypeCalculated, LDRCalculatedPct: f(calc), LDRAdjustmentPct: f(adj)})
	return e
}

func TestSetActualLDR_ClearsAdjustment_ReaderReturnsActual(t *testing.T) {
	e := spinWithAdj(1.60, -0.15)
	assert.NoError(t, e.SetActualLDR(1.52, "sync"))
	assert.Nil(t, e.LDRAdjustmentPct())
	assert.InDelta(t, 1.52, *e.LDRCalculatedPct(), 1e-9)
	got, ok := mbspin.NumericFillReaders["mbs_run_ldr_pct"](e)
	assert.True(t, ok)
	assert.InDelta(t, 1.52, got, 1e-9)
}

func TestLockLDRActual_KeepsAdjustment_ReaderReturnsCalcPlusAdj(t *testing.T) {
	e := spinWithAdj(1.60, -0.15)
	e.LockLDRActual()
	assert.NotNil(t, e.LDRAdjustmentPct())
	got, ok := mbspin.NumericFillReaders["mbs_run_ldr_pct"](e)
	assert.True(t, ok)
	assert.InDelta(t, 1.45, got, 1e-9)
}
