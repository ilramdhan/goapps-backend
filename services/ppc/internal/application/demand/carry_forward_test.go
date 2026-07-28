package demand_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// stubRepo is a minimal in-memory demand.Repository for carry-forward tests.
type stubRepo struct {
	source  *demanddomain.Demand
	created []*demanddomain.Demand
	updated []*demanddomain.Demand
	nextID  int64
}

func (r *stubRepo) Create(_ context.Context, e *demanddomain.Demand) error {
	r.nextID++
	setID(e, r.nextID)
	r.created = append(r.created, e)
	return nil
}
func (r *stubRepo) GetByID(_ context.Context, _ int64) (*demanddomain.Demand, error) {
	return r.source, nil
}
func (r *stubRepo) List(_ context.Context, _ demanddomain.ListFilter) ([]*demanddomain.Demand, int64, error) {
	return nil, 0, nil
}
func (r *stubRepo) Update(_ context.Context, e *demanddomain.Demand) error {
	r.updated = append(r.updated, e)
	return nil
}
func (r *stubRepo) Delete(_ context.Context, _ int64) error { return nil }
func (r *stubRepo) CountPlanItems(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (r *stubRepo) ListCarryCandidates(_ context.Context, _ string) ([]*demanddomain.Demand, error) {
	return nil, nil
}
func (r *stubRepo) GetStagingByIDs(_ context.Context, _ []int64) ([]*demanddomain.SalesOrderStaging, error) {
	return nil, nil
}
func (r *stubRepo) ListStaging(_ context.Context, _ demanddomain.StagingListFilter) ([]*demanddomain.SalesOrderStaging, int64, error) {
	return nil, 0, nil
}
func (r *stubRepo) ListStagingIDs(_ context.Context, _ demanddomain.StagingIDsFilter, _ int) ([]int64, int64, error) {
	return nil, 0, nil
}
func (r *stubRepo) LookupStagingItemCodes(_ context.Context, _ []int64) (map[int64]string, error) {
	return nil, nil
}
func (r *stubRepo) MarkStagingPulled(_ context.Context, _, _ int64) error { return nil }
func (r *stubRepo) SetStagingProduct(_ context.Context, _, _ int64) (*demanddomain.SalesOrderStaging, error) {
	return nil, nil
}
func (r *stubRepo) ListUnresolvedStagingPairs(_ context.Context) ([]demanddomain.StagingPair, error) {
	return nil, nil
}
func (r *stubRepo) ApplyStagingResolutions(_ context.Context, _ []demanddomain.ProductResolution) (int64, error) {
	return 0, nil
}

// setID rebuilds a demand with an assigned id (fields are private).
func setID(e *demanddomain.Demand, id int64) {
	*e = *demanddomain.Reconstruct(demanddomain.ReconstructParams{
		ID:              id,
		Type:            e.Type(),
		SubType:         e.SubType(),
		Source:          e.Source(),
		CarryAction:     e.CarryAction(),
		CpmProductSysID: e.CpmProductSysID(),
		QtyOriginal:     e.QtyOriginal(),
		QtyRemaining:    e.QtyRemaining(),
		Deadline:        e.Deadline(),
		GradeReq:        e.GradeReq(),
		CarryFromID:     e.CarryFromID(),
		Status:          e.Status(),
		Month:           e.Month(),
		CreatedBy:       e.CreatedBy(),
		CreatedAt:       e.CreatedAt(),
		UpdatedAt:       e.UpdatedAt(),
	})
}

func confirmedSource(t *testing.T, remaining float64) *demanddomain.Demand {
	t.Helper()
	d := demanddomain.Reconstruct(demanddomain.ReconstructParams{
		ID:              1,
		Type:            demanddomain.TypeContract,
		SubType:         demanddomain.SubTypeNewExport,
		Source:          demanddomain.SourceManual,
		CpmProductSysID: 100,
		QtyOriginal:     remaining,
		QtyRemaining:    remaining,
		Deadline:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		GradeReq:        demanddomain.GradeReqNone,
		Status:          demanddomain.StatusConfirmed,
		Month:           "2026-08",
		CreatedBy:       1,
	})
	return d
}

func TestProcessCarryForward_Split_WithinRemaining(t *testing.T) {
	repo := &stubRepo{source: confirmedSource(t, 1000)}
	svc := demandapp.NewService(repo, nil, nil)

	deadline := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	created, err := svc.ProcessCarryForward(context.Background(), demandapp.ProcessCarryForwardCommand{
		SourceDemandID: 1,
		Action:         demanddomain.CarryActionSplit,
		TargetMonth:    "2026-09",
		Splits: []demandapp.CarryForwardSplit{
			{Qty: 600, Deadline: deadline},
			{Qty: 400, Deadline: deadline},
		},
		ActedBy: 3,
	})
	require.NoError(t, err)
	assert.Len(t, created, 2)
	for _, c := range created {
		assert.Equal(t, demanddomain.SourceCarryForward, c.Source())
		require.NotNil(t, c.CarryFromID())
		assert.Equal(t, int64(1), *c.CarryFromID())
	}
	assert.Equal(t, demanddomain.StatusSplit, repo.source.Status())
}

func TestProcessCarryForward_Split_ExceedsRemaining(t *testing.T) {
	repo := &stubRepo{source: confirmedSource(t, 1000)}
	svc := demandapp.NewService(repo, nil, nil)

	deadline := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.ProcessCarryForward(context.Background(), demandapp.ProcessCarryForwardCommand{
		SourceDemandID: 1,
		Action:         demanddomain.CarryActionSplit,
		TargetMonth:    "2026-09",
		Splits: []demandapp.CarryForwardSplit{
			{Qty: 700, Deadline: deadline},
			{Qty: 400, Deadline: deadline}, // sum 1100 > 1000
		},
		ActedBy: 3,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, demanddomain.ErrSplitExceedsRemaining)
	assert.EqualError(t, err, "ppc: split qty melebihi remaining qty demand")
}

func TestProcessCarryForward_AsIs_CarriesOver(t *testing.T) {
	repo := &stubRepo{source: confirmedSource(t, 500)}
	svc := demandapp.NewService(repo, nil, nil)

	created, err := svc.ProcessCarryForward(context.Background(), demandapp.ProcessCarryForwardCommand{
		SourceDemandID: 1,
		Action:         demanddomain.CarryActionAsIs,
		TargetMonth:    "2026-09",
		ActedBy:        3,
	})
	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.InDelta(t, 500.0, created[0].QtyOriginal(), 1e-9)
	assert.Equal(t, demanddomain.SubTypeCFExport, created[0].SubType())
	assert.Equal(t, demanddomain.StatusCarriedOver, repo.source.Status())
}
