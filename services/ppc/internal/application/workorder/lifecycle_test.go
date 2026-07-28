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

// memRepo is an in-memory WO repository covering the child-collection operations
// exercised by the reference + adjust usecases.
type memRepo struct {
	woStubRepo
	seq        int64
	orders     map[int64]*workorderdomain.WorkOrder
	params     map[int64][]*workorderdomain.Parameter
	allocs     map[int64][]*workorderdomain.RmAllocation
	adjustCall int
}

func newMemRepo() *memRepo {
	return &memRepo{
		orders: map[int64]*workorderdomain.WorkOrder{},
		params: map[int64][]*workorderdomain.Parameter{},
		allocs: map[int64][]*workorderdomain.RmAllocation{},
	}
}

func (r *memRepo) Create(_ context.Context, e *workorderdomain.WorkOrder) error {
	r.seq++
	e.SetID(r.seq)
	r.orders[r.seq] = e
	return nil
}

func (r *memRepo) GetByID(_ context.Context, id int64) (*workorderdomain.WorkOrder, error) {
	wo, ok := r.orders[id]
	if !ok {
		return nil, workorderdomain.ErrNotFound
	}
	wo.AttachParameters(r.params[id])
	wo.AttachRmAllocations(r.allocs[id])
	return wo, nil
}

func (r *memRepo) Update(_ context.Context, e *workorderdomain.WorkOrder) error {
	r.orders[e.ID()] = e
	return nil
}

func (r *memRepo) ReplaceParameters(_ context.Context, woID int64, ps []*workorderdomain.Parameter) error {
	r.params[woID] = ps
	return nil
}

func (r *memRepo) ListParameters(_ context.Context, woID int64) ([]*workorderdomain.Parameter, error) {
	return r.params[woID], nil
}

func (r *memRepo) ReplaceRmAllocations(_ context.Context, woID int64, a []*workorderdomain.RmAllocation) error {
	r.allocs[woID] = a
	return nil
}

func (r *memRepo) ListRmAllocations(_ context.Context, woID int64) ([]*workorderdomain.RmAllocation, error) {
	return r.allocs[woID], nil
}

func (r *memRepo) AdjustActual(_ context.Context, woID int64, date time.Time, shift string, qty float64, reason string, editedBy int64) (*workorderdomain.ProductionActual, error) {
	r.adjustCall++
	return &workorderdomain.ProductionActual{
		WOID: woID, Date: date, Shift: shift,
		QtyBobbin: 1000, QtyActual: qty, QtySource: "ADJUSTED", AdjustReason: reason,
		LastEditedBy: &editedBy,
	}, nil
}

func seedSourceWO(t *testing.T, r *memRepo) *workorderdomain.WorkOrder {
	t.Helper()
	src, err := workorderdomain.New(workorderdomain.NewParams{
		WoNo: "WO-SRC", LotNo: "SRC1", AreaCode: "TXT", MachineID: 2,
		CrhHeadID: 10, CrhVersion: 1, PlanItemID: 7, QtyTarget: 500,
		Deadline: time.Now().Add(48 * time.Hour), CreatedBy: 1,
		DemandID: ptrI64(99),
	})
	require.NoError(t, err)
	require.NoError(t, r.Create(context.Background(), src))
	r.params[src.ID()] = []*workorderdomain.Parameter{
		{WOID: src.ID(), ParamID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", ParamCode: "SPEED", DataType: "NUMBER", IsDual: true, ValuePPCNum: fptr(700)},
	}
	return src
}

func ptrI64(v int64) *int64   { return &v }
func fptr(v float64) *float64 { return &v }

func TestCreateWOReference_Template_NoLinkCopiesParams(t *testing.T) {
	r := newMemRepo()
	src := seedSourceWO(t, r)
	svc := newSvc(r) // no resolver → direct copy path

	newWO, err := svc.CreateWOReference(context.Background(), workorderapp.CreateWOReferenceCommand{
		SourceWOID: src.ID(),
		RefType:    workorderdomain.RefTypeTemplate,
		LotNo:      "TPL1",
		QtyTarget:  600,
		Deadline:   time.Now().Add(72 * time.Hour),
		CreatedBy:  3,
	})
	require.NoError(t, err)
	assert.Nil(t, newWO.RefWoID(), "TEMPLATE must not bind to source")
	assert.Equal(t, workorderdomain.RefTypeTemplate, newWO.RefType())
	assert.Nil(t, newWO.DemandID(), "TEMPLATE must not inherit demand")
	require.Len(t, newWO.Parameters(), 1)
	assert.Equal(t, 700.0, *newWO.Parameters()[0].ValuePPCNum)
}

func TestCreateWOReference_Continuation_LinksAndInheritsDemand(t *testing.T) {
	r := newMemRepo()
	src := seedSourceWO(t, r)
	svc := newSvc(r)

	newWO, err := svc.CreateWOReference(context.Background(), workorderapp.CreateWOReferenceCommand{
		SourceWOID: src.ID(),
		RefType:    workorderdomain.RefTypeContinuation,
		LotNo:      "CONT1",
		QtyTarget:  200,
		Deadline:   time.Now().Add(72 * time.Hour),
		CreatedBy:  3,
	})
	require.NoError(t, err)
	require.NotNil(t, newWO.RefWoID())
	assert.Equal(t, src.ID(), *newWO.RefWoID())
	assert.Equal(t, workorderdomain.RefTypeContinuation, newWO.RefType())
	require.NotNil(t, newWO.DemandID())
	assert.Equal(t, int64(99), *newWO.DemandID())
}

func TestCreateWOReference_InvalidRefType(t *testing.T) {
	r := newMemRepo()
	src := seedSourceWO(t, r)
	svc := newSvc(r)
	_, err := svc.CreateWOReference(context.Background(), workorderapp.CreateWOReferenceCommand{
		SourceWOID: src.ID(), RefType: "NOPE", LotNo: "X", QtyTarget: 1, Deadline: time.Now().Add(time.Hour),
	})
	assert.ErrorIs(t, err, workorderdomain.ErrInvalidRefType)
}

func TestAdjustWOActual_TwoAxis(t *testing.T) {
	r := newMemRepo()
	svc := newSvc(r)
	actual, err := svc.AdjustWOActual(context.Background(), workorderapp.AdjustWOActualCommand{
		WOID: 1, Date: time.Now(), Shift: "1", QtyActual: 950, Reason: "recount", EditedBy: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, r.adjustCall)
	assert.Equal(t, "ADJUSTED", actual.QtySource)
	assert.Equal(t, 950.0, actual.QtyActual)
	assert.Equal(t, 1000.0, actual.QtyBobbin, "bobbin baseline preserved")
}

func TestAdjustWOActual_RequiresReason(t *testing.T) {
	svc := newSvc(newMemRepo())
	_, err := svc.AdjustWOActual(context.Background(), workorderapp.AdjustWOActualCommand{WOID: 1, QtyActual: 1})
	assert.ErrorIs(t, err, workorderdomain.ErrEmptyReason)
}

func TestSaveWOParameters_And_PCApproveMaterialization(t *testing.T) {
	r := newMemRepo()
	src := seedSourceWO(t, r)
	svc := newSvc(r)
	require.NoError(t, r.orders[src.ID()].Submit())

	// PC approve with an explicit PC value override.
	_, err := svc.ApproveWOParameter(context.Background(), src.ID(), []workorderapp.ParamValueInput{
		{ParamID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Num: fptr(680)},
	}, 4)
	require.NoError(t, err)
	assert.Equal(t, workorderdomain.StatusPCApproved, r.orders[src.ID()].Status())
	assert.Equal(t, 680.0, *r.params[src.ID()][0].ValuePCNum)
}
