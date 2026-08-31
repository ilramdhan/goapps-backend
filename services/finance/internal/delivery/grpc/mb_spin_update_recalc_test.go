package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// fakeUpdateRecalcRepo is a minimal mbspin.RecalcRepository fake for proving
// UpdateMBSpin surfaces cascade data (P7-T5). It reports one direct child that
// is left alone (A7 — a Spinning-status child holds an actual value).
type fakeUpdateRecalcRepo struct {
	children []*mbspin.Entity
}

func (f *fakeUpdateRecalcRepo) ListAllChildren(_ context.Context, _ uuid.UUID) ([]*mbspin.Entity, error) {
	return f.children, nil
}

func (f *fakeUpdateRecalcRepo) ApplyChildRecalc(_ context.Context, _ mbspin.RecalcApplyInput) error {
	return nil
}

// fakeUpdateImpactRepo is a minimal mbdozing.ImpactRepository fake returning a
// canned READ-ONLY impact preview — it performs no calculation, mirroring the
// real repository, which only SELECTs.
type fakeUpdateImpactRepo struct {
	rows   []mbdozing.ImpactRow
	totals mbdozing.Totals
}

func (f *fakeUpdateImpactRepo) ImpactBySpin(_ context.Context, _ string, _ int) ([]mbdozing.ImpactRow, mbdozing.Totals, error) {
	return f.rows, f.totals, nil
}

// fakeUpdateFactorRepo satisfies mbcrosssection.FactorRepository via
// embedding + GetByPair override, same pattern as the application-layer
// recalc tests. The LDR cascade never reaches a real lookup in this test (no
// cross section is configured), but the collaborator must be non-nil.
type fakeUpdateFactorRepo struct {
	mbcrosssection.FactorRepository
}

func (f *fakeUpdateFactorRepo) GetByPair(_ context.Context, _, _ string) (*mbcrosssection.FactorEntity, error) {
	return nil, nil //nolint:nilnil // test double: "no factor configured" is expressed as nil
}

// newSpinForRecalc builds a spin entity with the denier/filament/dozing/status
// fields a recalc pass reads, plus an ORION item code so the D24 impact
// preview attaches.
func newSpinForRecalc(t *testing.T, id uuid.UUID, orionCode string, denier float64, filament int, dozing float64, status *string) *mbspin.Entity {
	t.Helper()
	e := mbspin.Reconstruct(
		id, nil, &orionCode, uuid.New(), "SPIN-PARENT",
		&denier, &filament, &dozing, nil,
		nil, nil,
		status, nil, nil, nil,
		nil, nil,
		true, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "tester", nil, nil, nil, nil,
	)
	return e
}

// TestUpdateMBSpin_SurfacesCascadeResult proves P7-T5: UpdateMBSpin's gRPC
// response now carries the skip list and D24 impact preview when a
// denier/filament/dozing change cascades to the parent's children, instead of
// discarding the HandleWithRecalc result.
func TestUpdateMBSpin_SurfacesCascadeResult(t *testing.T) {
	parentID := uuid.New()
	rndStatus := mbspin.StatusRnD
	spinningStatus := "Spinning"

	parent := newSpinForRecalc(t, parentID, "ORION-1", 380, 108, 0.9, &rndStatus)
	skippedChild := newSpinForRecalc(t, uuid.New(), "", 500, 96, 0.5, &spinningStatus)

	repo := &captureRepo{stored: parent}
	recalcRepo := &fakeUpdateRecalcRepo{children: []*mbspin.Entity{skippedChild}}
	impactRepo := &fakeUpdateImpactRepo{
		rows: []mbdozing.ImpactRow{
			{ProductSysID: 1, ProductCode: "P-1", ProductName: "Product 1", IsLocked: true},
		},
		totals: mbdozing.Totals{TotalAffected: 3, TotalLocked: 1},
	}
	factorRepo := &fakeUpdateFactorRepo{}

	h, err := NewMBSpinHandlerWithRecalc(repo, recalcRepo, impactRepo, factorRepo)
	require.NoError(t, err)

	newDenier := 400.0
	resp, err := h.UpdateMBSpin(context.Background(), &financev1.UpdateMBSpinRequest{
		MbhId:     uuid.New().String(),
		MbsId:     parentID.String(),
		MbsDenier: &newDenier,
	})
	require.NoError(t, err)
	require.True(t, resp.Base.IsSuccess, resp.Base.Message)

	require.Len(t, resp.Skipped, 1, "the Spinning-status child must be reported, not silently dropped")
	assert.Equal(t, skippedChild.ID().String(), resp.Skipped[0].MbsId)
	assert.Equal(t, int32(1), resp.SkippedCount)

	require.Len(t, resp.ImpactPreview, 1)
	assert.Equal(t, "P-1", resp.ImpactPreview[0].CpmProductCode)
	assert.Equal(t, int32(3), resp.ImpactTotalAffected)
	assert.Equal(t, int32(1), resp.ImpactTotalLocked)
}

// TestUpdateMBSpin_NoCascade_ResponseStaysEmpty proves the common case — an
// update that does not touch denier/filament/dozing, or a handler built
// without recalc wiring — leaves the new fields at their zero value instead of
// fabricating cascade data that never ran.
func TestUpdateMBSpin_NoCascade_ResponseStaysEmpty(t *testing.T) {
	repo := &captureRepo{stored: newSpinWithFlags(t, nil, nil)}
	h, err := NewMBSpinHandler(repo)
	require.NoError(t, err)

	resp, err := h.UpdateMBSpin(context.Background(), &financev1.UpdateMBSpinRequest{
		MbhId: uuid.New().String(),
		MbsId: uuid.New().String(),
	})
	require.NoError(t, err)
	require.True(t, resp.Base.IsSuccess, resp.Base.Message)

	assert.Empty(t, resp.Skipped)
	assert.Zero(t, resp.SkippedCount)
	assert.Empty(t, resp.ImpactPreview)
	assert.Zero(t, resp.ImpactTotalAffected)
	assert.Zero(t, resp.ImpactTotalLocked)
	assert.False(t, resp.ImpactTruncated)
}
