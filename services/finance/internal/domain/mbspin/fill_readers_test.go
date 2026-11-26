package mbspin_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func f(v float64) *float64 { return &v }

// mb-comparison-p2: a confirmed Actual LDR drops the stale manual adjustment, so the
// effective LDR equals run_ldr, which MB_SP_DOZING reads raw.
func TestSetActualLDR_ClearsAdjustment_ReaderReturnsActual(t *testing.T) {
	e := mbspin.Reconstruct(uuid.New(), nil, nil, uuid.New(), "x", nil, nil, nil, nil, nil, nil, nil, nil, f(1.60), nil, nil, nil, true, time.Now(), "t", nil, nil, nil, nil)
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{LDRType: mbspin.LDRTypeCalculated, LDRCalculatedPct: f(1.60), LDRAdjustmentPct: f(-0.15)})
	assert.NoError(t, e.SetActualLDR(1.45, "sync"))
	assert.Nil(t, e.LDRAdjustmentPct())
	assert.InDelta(t, 1.45, *e.LDRCalculatedPct(), 1e-9)
	got, ok := mbspin.NumericFillReaders["mbs_run_ldr_pct"](e)
	assert.True(t, ok)
	assert.InDelta(t, 1.45, got, 1e-9)
}
