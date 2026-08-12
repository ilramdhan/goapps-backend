package mbbatch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

func TestBatchCosts_OverlayPrefersInBatchValueOverCommitted(t *testing.T) {
	costs := newBatchCosts([]MBHeadCandidate{{MBHID: "child", CostProductID: 40418}})
	costs.publish(40418, map[costcalcdom.CalculationType]float64{
		costcalcdom.CalcTypeActual: 12.5,
	})

	// The loader's map is the PREVIOUS run's committed value.
	got := map[int64]float64{40418: 9.0}
	costs.overlay(got, costcalcdom.CalcTypeActual, []int64{40418})

	require.InDelta(t, 12.5, got[40418], 1e-9,
		"the cost computed in this run must win over the previously committed one")
}

func TestBatchCosts_OverlayIsPerCalcType(t *testing.T) {
	costs := newBatchCosts([]MBHeadCandidate{{CostProductID: 7}})
	costs.publish(7, map[costcalcdom.CalculationType]float64{
		costcalcdom.CalcTypeActual:  1,
		costcalcdom.CalcTypeSelling: 2,
	})

	actual := map[int64]float64{}
	costs.overlay(actual, costcalcdom.CalcTypeActual, []int64{7})
	selling := map[int64]float64{}
	costs.overlay(selling, costcalcdom.CalcTypeSelling, []int64{7})
	forecast := map[int64]float64{}
	costs.overlay(forecast, costcalcdom.CalcTypeForecast, []int64{7})

	require.InDelta(t, 1.0, actual[7], 1e-9)
	require.InDelta(t, 2.0, selling[7], 1e-9)
	require.Empty(t, forecast, "an unpublished calc type must not borrow another type's value")
}

// A child whose MB Head is not VALIDATED is never a candidate, so nothing is ever published
// for it and the committed value must survive untouched.
func TestBatchCosts_OverlayLeavesUnscheduledChildAlone(t *testing.T) {
	costs := newBatchCosts([]MBHeadCandidate{{CostProductID: 100}})

	got := map[int64]float64{200: 3.75}
	costs.overlay(got, costcalcdom.CalcTypeActual, []int64{200})

	require.InDelta(t, 3.75, got[200], 1e-9)
	require.False(t, costs.scheduled(200))
	require.True(t, costs.scheduled(100))
}

// loadUpstreamCosts is where the cache meets the loader. Proven without a database: the
// loader returns nothing (no committed row yet) and the value still reaches the parent.
func TestLoadUpstreamCosts_InBatchChildReachesParentWithNoCommittedRow(t *testing.T) {
	svc := &Service{loader: &fakeLoader{upstream: map[int64]float64{}}}
	costs := newBatchCosts([]MBHeadCandidate{{MBHID: "child", CostProductID: 40418}})
	costs.publish(40418, map[costcalcdom.CalculationType]float64{costcalcdom.CalcTypeActual: 12.451513})

	got, err := svc.loadUpstreamCosts(context.Background(), upstreamRequest{
		products: []int64{40418},
		period:   "202607",
		calcType: costcalcdom.CalcTypeActual,
		parent:   MBHeadCandidate{MBHID: "parent", Code: "MB-PARENT", CostProductID: 40895},
		jobID:    4242,
		costs:    costs,
	})
	require.NoError(t, err)
	require.InDelta(t, 12.451513, got[40418], 1e-9,
		"a child computed in this same run must be visible to its parent even though its row is still uncommitted")
}

// The nil-map case: LoadUpstreamCosts is allowed to return nil, and the overlay must not
// panic writing into it.
func TestLoadUpstreamCosts_HandlesNilLoaderMap(t *testing.T) {
	svc := &Service{loader: &fakeLoader{upstream: nil}}
	costs := newBatchCosts([]MBHeadCandidate{{CostProductID: 5}})
	costs.publish(5, map[costcalcdom.CalculationType]float64{costcalcdom.CalcTypeActual: 2.5})

	got, err := svc.loadUpstreamCosts(context.Background(), upstreamRequest{
		products: []int64{5},
		period:   "202607",
		calcType: costcalcdom.CalcTypeActual,
		costs:    costs,
	})
	require.NoError(t, err)
	require.InDelta(t, 2.5, got[5], 1e-9)
}
