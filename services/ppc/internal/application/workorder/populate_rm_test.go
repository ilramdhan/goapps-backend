package workorder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// fakePlanItems resolves a fixed product sys id for any plan item.
type fakePlanItems struct{ productSysID int64 }

func (f fakePlanItems) ProductSysID(context.Context, int64) (int64, error) {
	return f.productSysID, nil
}

// fakeRouteRms returns fixed route RM components.
type fakeRouteRms struct {
	comps []workorderdomain.RouteRmComponent
}

func (f fakeRouteRms) RouteRmComponents(context.Context, int64) ([]workorderdomain.RouteRmComponent, error) {
	return f.comps, nil
}

// errRouteRms simulates an unreachable finance for the route lookup.
type errRouteRms struct{ err error }

func (f errRouteRms) RouteRmComponents(context.Context, int64) ([]workorderdomain.RouteRmComponent, error) {
	return nil, f.err
}

func seedPlainWO(t *testing.T, r *memRepo, qtyTarget float64) *workorderdomain.WorkOrder {
	t.Helper()
	wo, err := workorderdomain.New(workorderdomain.NewParams{
		WoNo: "WO-RM", LotNo: "RM1", AreaCode: "TXT", MachineID: 2,
		CrhHeadID: 10, CrhVersion: 1, PlanItemID: 7, QtyTarget: qtyTarget,
		Deadline: time.Now().Add(48 * time.Hour), CreatedBy: 1,
	})
	require.NoError(t, err)
	require.NoError(t, r.Create(context.Background(), wo))
	return wo
}

func TestPopulateRmFromRoute_MaterializesRatioQty(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 1000)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms: fakeRouteRms{comps: []workorderdomain.RouteRmComponent{
			{CrmRmID: 101, RmType: "PRODUCT", ShadeCode: "SH1", Ratio: 0.6}, // POY genealogy
			{CrmRmID: 102, RmType: "ITEM", ShadeCode: "SH2", Ratio: 0.4},
		}},
	})

	got, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID()})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, int64(101), got[0].CrmRmID)
	assert.Equal(t, "PRODUCT", got[0].RmType)
	assert.Equal(t, "SH1", got[0].ShadeCode)
	assert.InDelta(t, 600.0, got[0].QtyAllocated, 1e-9) // 0.6 × 1000
	assert.InDelta(t, 400.0, got[1].QtyAllocated, 1e-9) // 0.4 × 1000

	// Preview mode must not persist.
	stored, err := r.ListRmAllocations(context.Background(), wo.ID())
	require.NoError(t, err)
	assert.Empty(t, stored)
}

func TestPopulateRmFromRoute_ReplacePersists(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 500)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms:  fakeRouteRms{comps: []workorderdomain.RouteRmComponent{{CrmRmID: 7, RmType: "ITEM", Ratio: 1.0}}},
	})

	got, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID(), Replace: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.InDelta(t, 500.0, got[0].QtyAllocated, 1e-9)

	stored, err := r.ListRmAllocations(context.Background(), wo.ID())
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, int64(7), stored[0].CrmRmID)
}

func TestPopulateRmFromRoute_NoRouteSourceReturnsStored(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 500)
	svc := workorderapp.NewService(r, workorderapp.Deps{}) // no routeRms / planItems

	got, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID()})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// A product with no released route is a normal outcome, not an error: the panel
// shows an explanatory empty state and manual lines stay available.
func TestPopulateRmFromRoute_UnroutedProductIsEmptyNotError(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 500)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms:  fakeRouteRms{comps: nil}, // finance answers "no released route"
	})

	got, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID()})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Suggestions carry the labels finance resolved, so no consumer ever needs to
// render crm_rm_id, and each line can be attributed to its route stage.
func TestPopulateRmFromRoute_CarriesResolvedLabelsAndStage(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 200)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms: fakeRouteRms{comps: []workorderdomain.RouteRmComponent{
			{
				CrmRmID: 101, RmType: "ITEM", Ratio: 0.25,
				RmCode: "CHIPS_SD", RmName: "Chips Semi Dull",
				RouteStageName: "Spinning", RouteLevel: 1,
			},
		}},
	})

	got, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID()})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "CHIPS_SD", got[0].RmCode)
	assert.Equal(t, "Chips Semi Dull", got[0].RmName)
	assert.Equal(t, "Spinning", got[0].RouteStageName)
	assert.Equal(t, int32(1), got[0].RouteLevel)
	assert.InDelta(t, 0.25, got[0].RouteRmRatio, 1e-9)
	assert.InDelta(t, 50.0, got[0].QtyAllocated, 1e-9) // 0.25 × 200
}

// A multi-stage route contributes RM lines from every stage, each attributed to
// the stage it came from.
func TestPopulateRmFromRoute_MultiStageCoversEveryStage(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 100)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms: fakeRouteRms{comps: []workorderdomain.RouteRmComponent{
			{CrmRmID: 1, RmType: "ITEM", Ratio: 1.0, RmCode: "CHIPS", RouteStageName: "Spinning", RouteLevel: 1},
			{CrmRmID: 2, RmType: "PRODUCT", Ratio: 0.5, RmCode: "POY-150", RouteStageName: "Texturizing", RouteLevel: 2},
		}},
	})

	got, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID()})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Spinning", got[0].RouteStageName)
	assert.Equal(t, "Texturizing", got[1].RouteStageName)
	assert.InDelta(t, 100.0, got[0].QtyAllocated, 1e-9)
	assert.InDelta(t, 50.0, got[1].QtyAllocated, 1e-9)
}

// Finance unreachable surfaces as an error on the explicit populate call — the
// caller asked for a route and there is no honest empty answer to give.
func TestPopulateRmFromRoute_FinanceUnreachableErrors(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 500)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms:  errRouteRms{err: errors.New("finance unavailable")},
	})

	_, err := svc.PopulateRmFromRoute(context.Background(), workorderapp.PopulateRmFromRouteCommand{WOID: wo.ID()})
	require.Error(t, err)
}

// The save response itself is decorated, so the panel can name each RM the
// moment it saves rather than waiting for the next refetch. Covers the path
// that now reuses the entity read at the top of SaveWORmAllocations instead of
// issuing a second GetByID for the plan item.
func TestSaveWORmAllocations_ResponseIsDecorated(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 1000)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms: fakeRouteRms{comps: []workorderdomain.RouteRmComponent{
			{CrmRmID: 101, RmType: "ITEM", Ratio: 0.6, RmCode: "CHIPS_SD", RmName: "Chips Semi Dull", RouteStageName: "Spinning", RouteLevel: 1},
		}},
	})

	saved, err := svc.SaveWORmAllocations(context.Background(), workorderapp.SaveWORmAllocationsCommand{
		WOID: wo.ID(),
		Allocations: []workorderapp.RmAllocationInput{
			{CrmRmID: 101, RmType: "ITEM", LotNo: "L1", RmSource: "STORE", QtyAllocated: 600},
		},
	})
	require.NoError(t, err)
	require.Len(t, saved, 1)
	assert.Equal(t, "CHIPS_SD", saved[0].RmCode)
	assert.Equal(t, "Chips Semi Dull", saved[0].RmName)
	assert.Equal(t, "Spinning", saved[0].RouteStageName)
	assert.Equal(t, int32(1), saved[0].RouteLevel)
}

// Reading a WO decorates its stored allocations with route labels, so the
// detail table can name each RM. Lines whose route edge no longer exists keep
// their stored values and stay unlabeled rather than disappearing.
func TestGet_DecoratesStoredRmAllocations(t *testing.T) {
	r := newMemRepo()
	wo := seedPlainWO(t, r, 1000)
	svc := workorderapp.NewService(r, workorderapp.Deps{
		PlanItems: fakePlanItems{productSysID: 55},
		RouteRms: fakeRouteRms{comps: []workorderdomain.RouteRmComponent{
			{CrmRmID: 101, RmType: "ITEM", Ratio: 0.6, RmCode: "CHIPS_SD", RmName: "Chips Semi Dull", RouteStageName: "Spinning", RouteLevel: 1},
		}},
	})

	_, err := svc.SaveWORmAllocations(context.Background(), workorderapp.SaveWORmAllocationsCommand{
		WOID: wo.ID(),
		Allocations: []workorderapp.RmAllocationInput{
			{CrmRmID: 101, RmType: "ITEM", LotNo: "L1", RmSource: "STORE", QtyAllocated: 600},
			{CrmRmID: 999, RmType: "ITEM", LotNo: "L2", RmSource: "STORE", QtyAllocated: 10}, // not in route
		},
	})
	require.NoError(t, err)

	got, _, err := svc.Get(context.Background(), wo.ID())
	require.NoError(t, err)
	allocs := got.RmAllocations()
	require.Len(t, allocs, 2)
	assert.Equal(t, "CHIPS_SD", allocs[0].RmCode)
	assert.Equal(t, "Chips Semi Dull", allocs[0].RmName)
	assert.Equal(t, "Spinning", allocs[0].RouteStageName)
	// Off-route line survives, simply unlabeled.
	assert.Equal(t, int64(999), allocs[1].CrmRmID)
	assert.Empty(t, allocs[1].RmCode)
}
