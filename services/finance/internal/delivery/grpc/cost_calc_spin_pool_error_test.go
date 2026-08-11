package grpc

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// TestCostCalcErrToBase_MissingSpinFixedCost pins the status code for a missing
// POY spin pool.
//
// It is a configuration gap — Finance has not entered a mst_spin_fixed_cost row
// at or before the period — not a server fault, so it must surface as 400 with a
// message naming the period. As a 500 it reads as a backend crash and gets
// escalated to engineering instead of fixed in master data.
//
// The wrapped variants matter more than the bare one: in production the sentinel
// never arrives unwrapped. bulkLoad wraps it with the period, ProcessChunk's
// caller wraps that with "process chunk: ", so the mapper only ever sees the tail
// of a chain. A switch arm using == instead of errors.Is would pass on the bare
// case and fall through to 500 on every real one.
func TestCostCalcErrToBase_MissingSpinFixedCost(t *testing.T) {
	t.Parallel()

	const period = "202606"
	bare := costcalcdom.ErrMissingSpinFixedCost
	withPeriod := fmt.Errorf("%w: period %s", bare, period)
	asProduced := fmt.Errorf("process chunk: %w", withPeriod)

	tests := []struct {
		name       string
		err        error
		wantPeriod bool
	}{
		{name: "bare sentinel", err: bare},
		{name: "wrapped with period, as bulkLoad returns it", err: withPeriod, wantPeriod: true},
		{name: "double-wrapped, as the handler sees it", err: asProduced, wantPeriod: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := costCalcErrToBase(tt.err)
			require.NotNil(t, base)

			assert.Equal(t, "400", base.GetStatusCode(),
				"a missing spin pool is a master-data gap, not a server error")
			assert.False(t, base.GetIsSuccess())
			assert.Contains(t, base.GetMessage(), "spin fixed cost",
				"the message must say what is missing")
			if tt.wantPeriod {
				assert.Contains(t, base.GetMessage(), period,
					"the message must name the period finance has to fix")
			}
		})
	}
}

// TestMappedCostCalcErrToBase_MissingSpinPoolArmIsReachable guards the ordered
// switch. mappedCostCalcErrToBase matches top-down, so an earlier arm that the
// sentinel also satisfied would shadow the 400 mapping and this arm would become
// dead code that no status-code assertion could detect.
func TestMappedCostCalcErrToBase_MissingSpinPoolArmIsReachable(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: period 202606", costcalcdom.ErrMissingSpinFixedCost)

	base := mappedCostCalcErrToBase(err)
	require.NotNil(t, base, "the sentinel must be matched by the mapper, not fall through to default")
	assert.Equal(t, "400", base.GetStatusCode())

	// No other calc-engine sentinel may claim it on the way down the switch.
	for _, other := range []error{
		costcalcdom.ErrJobNotFound,
		costcalcdom.ErrCostNotFound,
		costcalcdom.ErrJobInvalidStatus,
		costcalcdom.ErrCostInvalidStatus,
		costcalcdom.ErrJobAlreadyRunning,
		costcalcdom.ErrCostAlreadyInFlight,
		costcalcdom.ErrInvalidPeriod,
		costcalcdom.ErrFormulaEval,
		costcalcdom.ErrCycleDetected,
	} {
		assert.False(t, errors.Is(err, other),
			"the missing-spin-pool error must not also satisfy %v", other)
	}

	// It must also not be picked up by the validation-string fallback, which
	// would reach 400 by accident and keep passing if the switch arm were deleted.
	assert.False(t, isCalcValidationErr(err),
		"the 400 must come from the sentinel arm, not the validation-string fallback")
}
