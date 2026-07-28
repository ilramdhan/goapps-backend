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

// woStubRepo is a minimal in-memory WO repository for auto-approve tests. Only
// the methods exercised by AutoApprovePending are meaningful.
type woStubRepo struct {
	pending []*workorderdomain.WorkOrder
	updated []*workorderdomain.WorkOrder
}

func (r *woStubRepo) Create(context.Context, *workorderdomain.WorkOrder) error { return nil }
func (r *woStubRepo) GetByID(context.Context, int64) (*workorderdomain.WorkOrder, error) {
	return nil, workorderdomain.ErrNotFound
}
func (r *woStubRepo) List(context.Context, workorderdomain.ListFilter) ([]*workorderdomain.WorkOrder, int64, error) {
	return nil, 0, nil
}
func (r *woStubRepo) Update(_ context.Context, e *workorderdomain.WorkOrder) error {
	r.updated = append(r.updated, e)
	return nil
}
func (r *woStubRepo) Delete(context.Context, int64) error { return nil }
func (r *woStubRepo) ReplacePlanItemLinks(context.Context, int64, []workorderdomain.PlanItemLink) error {
	return nil
}

func (r *woStubRepo) ListPlanItemLinks(context.Context, int64) ([]workorderdomain.PlanItemLink, error) {
	return nil, nil
}
func (r *woStubRepo) ReplaceParameters(context.Context, int64, []*workorderdomain.Parameter) error {
	return nil
}
func (r *woStubRepo) SetParameterPPCValue(context.Context, int64, *workorderdomain.Parameter) error {
	return nil
}
func (r *woStubRepo) SetParameterPCValue(context.Context, int64, *workorderdomain.Parameter) error {
	return nil
}
func (r *woStubRepo) ListParameters(context.Context, int64) ([]*workorderdomain.Parameter, error) {
	return nil, nil
}
func (r *woStubRepo) UpsertExecution(context.Context, *workorderdomain.Execution) error { return nil }
func (r *woStubRepo) ListExecutions(context.Context, int64) ([]*workorderdomain.Execution, error) {
	return nil, nil
}
func (r *woStubRepo) ReplaceRmAllocations(context.Context, int64, []*workorderdomain.RmAllocation) error {
	return nil
}
func (r *woStubRepo) ListRmAllocations(context.Context, int64) ([]*workorderdomain.RmAllocation, error) {
	return nil, nil
}
func (r *woStubRepo) GetProductionActuals(context.Context, int64, *time.Time, string) ([]*workorderdomain.ProductionActual, error) {
	return nil, nil
}
func (r *woStubRepo) AdjustActual(context.Context, int64, time.Time, string, float64, string, int64) (*workorderdomain.ProductionActual, error) {
	return nil, nil
}
func (r *woStubRepo) ListPendingApprovals(_ context.Context, _ time.Time) ([]*workorderdomain.WorkOrder, error) {
	return r.pending, nil
}
func (r *woStubRepo) MaxRevisionNo(context.Context, int64) (int32, error) { return 0, nil }

func submittedWO(t *testing.T) *workorderdomain.WorkOrder {
	t.Helper()
	wo, err := workorderdomain.New(workorderdomain.NewParams{
		WoNo:       "WO-TXT-1",
		LotNo:      "TXT1",
		AreaCode:   "TXT",
		MachineID:  2,
		CrhHeadID:  10,
		CrhVersion: 1,
		PlanItemID: 1,
		QtyTarget:  1000,
		Deadline:   time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		CreatedBy:  1,
	})
	require.NoError(t, err)
	require.NoError(t, wo.Submit())
	return wo
}

func newSvc(repo workorderdomain.Repository) *workorderapp.Service {
	return workorderapp.NewService(repo, workorderapp.Deps{})
}

func TestAutoApprovePending_ApprovesBothSides(t *testing.T) {
	wo := submittedWO(t)
	repo := &woStubRepo{pending: []*workorderdomain.WorkOrder{wo}}
	svc := newSvc(repo)

	res, err := svc.AutoApprovePending(context.Background(), time.Now(), 4*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Scanned)
	assert.Equal(t, 1, res.PCApproved)
	assert.Equal(t, 1, res.PMApproved)
	assert.Equal(t, 1, res.FullyApproved)
	assert.Equal(t, workorderdomain.StatusApproved, wo.Status())
}

func TestAutoApprovePending_OnlyPMWhenPCDone(t *testing.T) {
	wo := submittedWO(t)
	_, err := wo.ApprovePC(9, time.Now()) // PC already approved manually
	require.NoError(t, err)
	repo := &woStubRepo{pending: []*workorderdomain.WorkOrder{wo}}
	svc := newSvc(repo)

	res, err := svc.AutoApprovePending(context.Background(), time.Now(), 4*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, res.PCApproved)
	assert.Equal(t, 1, res.PMApproved)
	assert.Equal(t, 1, res.FullyApproved)
	assert.Equal(t, workorderdomain.StatusApproved, wo.Status())
}

func TestAutoApprovePending_SkipsWhenDisabled(t *testing.T) {
	wo, err := workorderdomain.New(workorderdomain.NewParams{
		WoNo: "WO-TXT-2", LotNo: "TXT2", AreaCode: "TXT", MachineID: 2,
		CrhHeadID: 10, CrhVersion: 1, PlanItemID: 1, QtyTarget: 1000,
		Deadline: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), CreatedBy: 1,
		AutoApproveDisabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, wo.Submit())
	repo := &woStubRepo{pending: []*workorderdomain.WorkOrder{wo}}
	svc := newSvc(repo)

	res, err := svc.AutoApprovePending(context.Background(), time.Now(), 4*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Scanned)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 0, res.FullyApproved)
	assert.Equal(t, workorderdomain.StatusSubmitted, wo.Status())
}

func TestAutoApprovePending_NoCandidates(t *testing.T) {
	repo := &woStubRepo{}
	svc := newSvc(repo)
	res, err := svc.AutoApprovePending(context.Background(), time.Now(), 4*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Scanned)
}
