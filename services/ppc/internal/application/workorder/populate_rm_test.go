package workorder_test

import (
	"context"
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
