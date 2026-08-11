package costcalc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// =============================================================================
// Unit tests for the POY spin-pool guard in bulkLoad.
//
// Service.loader is the ProductLoader interface, so the three guard branches are
// reachable with a fake and no database. Only the methods bulkLoad actually calls
// are implemented; the rest come from the embedded nil interface and panic, so a
// new dependency added to bulkLoad fails loudly instead of silently returning a
// zero value.
//
// The suite deliberately locks BOTH halves of the contract. The block ("no row at
// or before the period is fatal") and the carry-forward ("an older row is a valid,
// warning-only case") are one decision, and a test set that only proved the block
// would still pass if someone tightened the loader's `msfc_period <= $1` to `= $1`
// — which is exactly the change that breaks every local and dev database whose
// only row is the 202604 anchor.
// =============================================================================

// spinFakeLoader answers every bulkLoad dependency with an empty result except
// LoadSpinFixedCost, which returns whatever the test wants.
type spinFakeLoader struct {
	ProductLoader
	pool    SpinPool
	poolErr error

	gotPeriod string
	calls     int
}

func (f *spinFakeLoader) LoadRoutesByProducts(_ context.Context, _ []int64) (map[int64]*costroute.Graph, error) {
	return map[int64]*costroute.Graph{}, nil
}

func (f *spinFakeLoader) LoadCAPP(_ context.Context, _ []int64) (map[int64]map[string]float64, error) {
	return map[int64]map[string]float64{}, nil
}

func (f *spinFakeLoader) LoadFormulas(_ context.Context, _ []int64) (map[int64][]Formula, error) {
	return map[int64][]Formula{}, nil
}

func (f *spinFakeLoader) LoadRMCosts(_ context.Context, _ []string, _, _ string) (map[string]float64, error) {
	return map[string]float64{}, nil
}

func (f *spinFakeLoader) LoadUpstreamCosts(_ context.Context, _ []int64, _, _ string) (map[int64]float64, error) {
	return map[int64]float64{}, nil
}

func (f *spinFakeLoader) LoadSellingSnapshots(_ context.Context, _ []int64, _ string) (map[int64]map[string]float64, error) {
	return map[int64]map[string]float64{}, nil
}

func (f *spinFakeLoader) LoadSpinFixedCost(_ context.Context, period string) (SpinPool, error) {
	f.calls++
	f.gotPeriod = period
	return f.pool, f.poolErr
}

// spinServiceWithLoader builds a Service carrying only the loader. bulkLoad
// touches no repository, so the rest stay nil on purpose: a future bulkLoad that
// starts writing would nil-panic here rather than pass quietly.
func spinServiceWithLoader(l ProductLoader) *Service {
	return NewService(nil, nil, nil, nil, nil, l, nil, nil, nil)
}

func spinChunkInput(period string) ProcessChunkInput {
	return ProcessChunkInput{
		JobID:    7,
		ChunkID:  3,
		Period:   period,
		CalcType: costcalcdom.CalcTypeActual,
		Products: []int64{101},
		Actor:    "spin-guard-test",
	}
}

// fullPool is a pool with all six scope keys set to distinct values, so an
// assertion on it cannot pass while the values are transposed.
func fullPool(period string) SpinPool {
	return SpinPool{
		Period: period,
		Values: map[string]float64{
			ScopeKeySpinCommonPOYDenier: 329.712,
			ScopeKeySpinPOYProduction:   3027153,
			ScopeKeySpinPowerMonth:      198634,
			ScopeKeySpinManpowerMonth:   275561,
			ScopeKeySpinOverheadsMonth:  46600,
			ScopeKeySpinConsSprsMonth:   54100,
		},
	}
}

func TestBulkLoad_SpinPoolGuard(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("connection reset by peer")

	tests := []struct {
		name    string
		period  string
		pool    SpinPool
		poolErr error

		wantErr         bool
		wantSentinel    error
		wantErrContains string
		wantPoolPeriod  string
	}{
		{
			// A DB failure must abort the chunk. Falling back to an empty pool would
			// turn a transient outage into silently understated POY fixed cost.
			name:            "loader error is fatal",
			period:          "202606",
			poolErr:         errBoom,
			wantErr:         true,
			wantErrContains: "load spin fixed cost",
		},
		{
			// No row at or before the period: fatal, and identifiable by sentinel so
			// the gRPC layer can map it to 400 rather than 500.
			name:         "empty period is fatal with ErrMissingSpinFixedCost",
			period:       "202606",
			pool:         SpinPool{},
			wantErr:      true,
			wantSentinel: costcalcdom.ErrMissingSpinFixedCost,
		},
		{
			// The exact-match path. Nothing stale, nothing to warn about.
			name:           "exact period match succeeds",
			period:         "202606",
			pool:           fullPool("202606"),
			wantErr:        false,
			wantPoolPeriod: "202606",
		},
		{
			// THE OTHER HALF OF THE CONTRACT. A pool carried forward from an older
			// month is legitimate — legacy MST_PARAM_DATA was current-only, and the
			// seeded 202604 anchor is the only row on most databases. This must stay
			// non-fatal, or every period after the newest master row stops costing.
			name:           "stale period is non-fatal carry-forward",
			period:         "202612",
			pool:           fullPool("202604"),
			wantErr:        false,
			wantPoolPeriod: "202604",
		},
		{
			// A pool NEWER than the requested period should never reach bulkLoad (the
			// loader's <= cutoff prevents it), but if it ever did it is still only a
			// staleness mismatch, not a hard failure — the guard blocks absence, not
			// disagreement.
			name:           "future pool period is non-fatal",
			period:         "202601",
			pool:           fullPool("202604"),
			wantErr:        false,
			wantPoolPeriod: "202604",
		},
		{
			// Degenerate but reachable: a row whose Values map is empty still has a
			// Period, so it is NOT the missing case. Only Period drives fatality.
			name:           "non-empty period with empty values is not the missing case",
			period:         "202606",
			pool:           SpinPool{Period: "202606", Values: map[string]float64{}},
			wantErr:        false,
			wantPoolPeriod: "202606",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &spinFakeLoader{pool: tt.pool, poolErr: tt.poolErr}
			svc := spinServiceWithLoader(fake)

			bundle, err := svc.bulkLoad(context.Background(), spinChunkInput(tt.period))

			assert.Equal(t, 1, fake.calls, "bulkLoad must consult the loader exactly once")
			assert.Equal(t, tt.period, fake.gotPeriod,
				"bulkLoad must request the chunk's period, not a derived one")

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, bundle, "no bundle may escape a fatal load")
				if tt.wantSentinel != nil {
					assert.ErrorIs(t, err, tt.wantSentinel)
					assert.Contains(t, err.Error(), tt.period,
						"the error must name the period finance has to fix")
				}
				if tt.wantErrContains != "" {
					assert.Contains(t, err.Error(), tt.wantErrContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, bundle)
			assert.Equal(t, tt.wantPoolPeriod, bundle.spinPool.Period)
			assert.Equal(t, tt.pool.Values, bundle.spinPool.Values,
				"the pool values must reach the bundle untouched")
		})
	}
}

// TestBulkLoad_SpinPoolMissing_NotConfusedWithOtherSentinels pins that the new
// sentinel is distinguishable. If ErrMissingSpinFixedCost ever gets wrapped in or
// aliased to another calc-engine error, the gRPC mapper's ordered switch would
// return the wrong status code for one of them.
func TestBulkLoad_SpinPoolMissing_NotConfusedWithOtherSentinels(t *testing.T) {
	t.Parallel()

	fake := &spinFakeLoader{pool: SpinPool{}}
	_, err := spinServiceWithLoader(fake).bulkLoad(context.Background(), spinChunkInput("202606"))
	require.Error(t, err)

	assert.ErrorIs(t, err, costcalcdom.ErrMissingSpinFixedCost)
	for _, other := range []error{
		costcalcdom.ErrFormulaEval,
		costcalcdom.ErrCycleDetected,
		costcalcdom.ErrChunkRetryExhausted,
		costcalcdom.ErrInvalidPeriod,
	} {
		assert.NotErrorIs(t, err, other)
	}
}

// TestBuildCalculationLog_RecordsSpinPoolPeriod covers the audit half of the
// change: a carried-forward pool has to be visible on the product row finance is
// looking at, not only in a chunk log line nobody reads.
func TestBuildCalculationLog_RecordsSpinPoolPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		poolPeriod string
	}{
		{"exact period", "202606"},
		{"carried-forward period", "202604"},
		{"empty period is still recorded", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := &ComputeOutput{CostPerUnit: 12.5, TotalRMCost: 7.5}
			b := buildCalculationLog(out, tt.poolPeriod)
			require.NotEmpty(t, b)

			var doc map[string]any
			require.NoError(t, json.Unmarshal(b, &doc))

			got, ok := doc["spin_pool_period"]
			require.True(t, ok, "spin_pool_period must be present in every calculation log")
			assert.Equal(t, tt.poolPeriod, got)
		})
	}
}
