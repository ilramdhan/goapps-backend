package costcalc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestHydrateResult_RoundTripsCostColumns guards the regression fixed in T2.3:
// HydrateResult silently dropped the 7 captive/delivery cost values, so every
// read path reported 0 no matter what was persisted.
func TestHydrateResult_RoundTripsCostColumns(t *testing.T) {
	now := time.Now()
	verified := now.Add(time.Hour)

	r := HydrateResult(
		101, 202, "202606", CalcTypeActual, 303, 4,
		1.5, 2.5, 3.5, 4.5, 7, "IDR",
		[]byte(`{"l":1}`), []byte(`{"rm":1}`), []byte(`{"p":1}`), []byte(`{"f":1}`),
		"hash-abc", ResultStatusVerified, 909, now, "alice",
		&verified, "bob",
		11.1, 22.2, 33.3, 44.4, 55.5, 66.6, 77.7,
	)

	assert.Equal(t, int64(101), r.ID())
	assert.Equal(t, int64(202), r.ProductSysID())
	assert.Equal(t, "202606", r.Period())
	assert.Equal(t, 4, r.Version())
	assert.Equal(t, "IDR", r.Currency())
	assert.Equal(t, "hash-abc", r.InputHash())
	assert.Equal(t, ResultStatusVerified, r.Status())
	assert.Equal(t, "bob", r.VerifiedBy())
	assert.Equal(t, &verified, r.VerifiedAt())

	// The 7 columns restored in T2.3.
	assert.InDelta(t, 11.1, r.CaptiveCost(), 1e-9)
	assert.InDelta(t, 22.2, r.DeliveryCost(), 1e-9)
	assert.InDelta(t, 33.3, r.VB1DelCost(), 1e-9)
	assert.InDelta(t, 44.4, r.VB2DelCost(), 1e-9)
	assert.InDelta(t, 55.5, r.VB3DelCost(), 1e-9)
	assert.InDelta(t, 66.6, r.VB4DelCost(), 1e-9)
	assert.InDelta(t, 77.7, r.VB5DelCost(), 1e-9)
}
