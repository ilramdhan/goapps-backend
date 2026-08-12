package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// =============================================================================
// Unit tests for the persist-site MB guard in ProcessChunk / computeOne.
//
// The orchestrator's loadProductRMEdges deliberately does NOT filter MB, so the MB
// exclusion constrains what a job TARGETS, not how dependencies are RESOLVED. That
// leaves one hole: a non-MB product with a PRODUCT-type RM edge to an MB would pull
// the MB into the DAG as a node, and persistResult would UpsertWithSupersede its
// cst_product_cost rows — re-creating the very bug the exclusion closed. The guard
// refuses at the WRITE site instead.
//
// The suite locks BOTH halves: MB is skipped, and every non-MB product in the same
// chunk is still computed and persisted. The second half is the regression guard for
// the standing "do not disturb yarn/POY/ACY" constraint.
// =============================================================================

// mbGuardLoader answers every bulkLoad dependency with a usable minimum: a route per
// requested product (so no product is blocked as MISSING_ROUTE), no formulas, and a
// non-empty spin pool (bulkLoad hard-blocks on an empty one).
type mbGuardLoader struct {
	ProductLoader
	products []int64
}

func (f *mbGuardLoader) LoadRoutesByProducts(_ context.Context, ids []int64) (map[int64]*costroute.Graph, error) {
	out := map[int64]*costroute.Graph{}
	for _, id := range ids {
		out[id] = &costroute.Graph{Head: &costroute.Head{HeadID: id * 10}}
	}
	return out, nil
}

func (f *mbGuardLoader) LoadCAPP(_ context.Context, _ []int64) (map[int64]map[string]float64, error) {
	return map[int64]map[string]float64{}, nil
}

func (f *mbGuardLoader) LoadFormulas(_ context.Context, _ []int64) (map[int64][]Formula, error) {
	return map[int64][]Formula{}, nil
}

func (f *mbGuardLoader) LoadRMCosts(_ context.Context, _ []string, _, _ string) (map[string]float64, error) {
	return map[string]float64{}, nil
}

func (f *mbGuardLoader) LoadUpstreamCosts(_ context.Context, _ []int64, _, _ string) (map[int64]float64, error) {
	return map[int64]float64{}, nil
}

func (f *mbGuardLoader) LoadSellingSnapshots(_ context.Context, _ []int64, _ string) (map[int64]map[string]float64, error) {
	return map[int64]map[string]float64{}, nil
}

func (f *mbGuardLoader) LoadSpinFixedCost(_ context.Context, period string) (SpinPool, error) {
	return fullPool(period), nil
}

// mbSetChecker reports a fixed set of MB product ids.
type mbSetChecker struct {
	mb    map[int64]bool
	err   error
	calls int
	asked []int64
}

func (c *mbSetChecker) MBProductIDs(_ context.Context, ids []int64) (map[int64]bool, error) {
	c.calls++
	c.asked = ids
	return c.mb, c.err
}

// recordingProductRepo captures the per-product terminal transitions.
type recordingProductRepo struct {
	costcalcdom.JobProductRepository
	success map[int64]bool
	blocked map[int64]string
	failed  map[int64]string
}

func newRecordingProductRepo() *recordingProductRepo {
	return &recordingProductRepo{
		success: map[int64]bool{},
		blocked: map[int64]string{},
		failed:  map[int64]string{},
	}
}

func (r *recordingProductRepo) MarkSuccess(_ context.Context, _, pid, _ int64, _ int, _ []byte) error {
	r.success[pid] = true
	return nil
}

func (r *recordingProductRepo) MarkBlocked(_ context.Context, _, pid int64, reason string, _ []byte) error {
	r.blocked[pid] = reason
	return nil
}

func (r *recordingProductRepo) MarkFailed(_ context.Context, _, pid int64, msg string, _ []byte) error {
	r.failed[pid] = msg
	return nil
}

// recordingResultRepo captures which products were persisted into cst_product_cost.
type recordingResultRepo struct {
	costcalcdom.ResultRepository
	upserted []int64
}

func (r *recordingResultRepo) UpsertWithSupersede(_ context.Context, res *costcalcdom.Result) (int64, int, float64, int64, error) {
	r.upserted = append(r.upserted, res.ProductSysID())
	return 1, 0, 0, 0, nil
}

// nopChunkRepo satisfies the chunk transitions ProcessChunk performs when ChunkID != 0.
// The suite passes ChunkID 0, so these are never called; the embedded nil interface
// would panic loudly if that ever changed.
type nopChunkRepo struct{ costcalcdom.ChunkRepository }

func mbGuardService(guard MBProductSetChecker, prodRepo *recordingProductRepo, resRepo *recordingResultRepo) *Service {
	return NewService(
		nil, &nopChunkRepo{}, prodRepo, resRepo, nil,
		&mbGuardLoader{}, evaluator.NewCache(), nil, nil,
		WithMBProductGuard(guard),
	)
}

func mbGuardInput(products []int64) ProcessChunkInput {
	return ProcessChunkInput{
		JobID:    42,
		ChunkID:  0,
		Period:   "202607",
		CalcType: costcalcdom.CalcTypeActual,
		Products: products,
		Actor:    "mb-persist-guard-test",
	}
}

// The core assertion: an MB dependency node in a mixed chunk is BLOCKED, never written,
// while every non-MB product in the same chunk computes and persists exactly as before.
func TestProcessChunk_MBDependencyNode_NotPersisted(t *testing.T) {
	t.Parallel()
	prodRepo := newRecordingProductRepo()
	resRepo := &recordingResultRepo{}
	guard := &mbSetChecker{mb: map[int64]bool{777: true}}
	svc := mbGuardService(guard, prodRepo, resRepo)

	out, err := svc.ProcessChunk(context.Background(), mbGuardInput([]int64{101, 777, 202}))
	require.NoError(t, err)

	// MB skipped, not written.
	require.NotContains(t, resRepo.upserted, int64(777))
	require.Equal(t, blockReasonMBOwnedByBatch, prodRepo.blocked[777])
	require.False(t, prodRepo.success[777])

	// Non-MB products in the SAME chunk are untouched by the guard.
	require.Contains(t, resRepo.upserted, int64(101))
	require.Contains(t, resRepo.upserted, int64(202))
	require.True(t, prodRepo.success[101])
	require.True(t, prodRepo.success[202])
	require.NotContains(t, prodRepo.blocked, int64(101))
	require.NotContains(t, prodRepo.blocked, int64(202))

	require.Equal(t, 2, out.Success)
	require.Equal(t, 1, out.Blocked)
	require.Equal(t, 0, out.Failed)
}

// A chunk with no MB at all must behave byte-for-byte as before: every product persists.
// This is the direct regression guard for yarn / POY / ACY.
func TestProcessChunk_AllNonMB_EveryProductPersisted(t *testing.T) {
	t.Parallel()
	prodRepo := newRecordingProductRepo()
	resRepo := &recordingResultRepo{}
	guard := &mbSetChecker{mb: map[int64]bool{}}
	svc := mbGuardService(guard, prodRepo, resRepo)

	products := []int64{11, 22, 33, 44}
	out, err := svc.ProcessChunk(context.Background(), mbGuardInput(products))
	require.NoError(t, err)

	require.ElementsMatch(t, products, resRepo.upserted)
	require.Empty(t, prodRepo.blocked)
	require.Empty(t, prodRepo.failed)
	require.Equal(t, len(products), out.Success)
	require.Equal(t, 1, guard.calls, "the guard must cost one query per chunk, not one per product")
	require.ElementsMatch(t, products, guard.asked)
}

// Without WithMBProductGuard the guard is disabled and nothing is skipped — existing
// call sites (tests, any wiring that omits it) keep their exact behavior.
func TestProcessChunk_NilGuard_NothingSkipped(t *testing.T) {
	t.Parallel()
	prodRepo := newRecordingProductRepo()
	resRepo := &recordingResultRepo{}
	svc := NewService(nil, &nopChunkRepo{}, prodRepo, resRepo, nil, &mbGuardLoader{}, evaluator.NewCache(), nil, nil)

	out, err := svc.ProcessChunk(context.Background(), mbGuardInput([]int64{5, 6}))
	require.NoError(t, err)

	require.ElementsMatch(t, []int64{5, 6}, resRepo.upserted)
	require.Equal(t, 2, out.Success)
	require.Empty(t, prodRepo.blocked)
}

// A guard failure must abort the chunk rather than degrade into "nothing is MB", which
// would silently re-open the write hole.
func TestProcessChunk_GuardError_AbortsChunk(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db down")
	prodRepo := newRecordingProductRepo()
	resRepo := &recordingResultRepo{}
	svc := mbGuardService(&mbSetChecker{err: sentinel}, prodRepo, resRepo)

	_, err := svc.ProcessChunk(context.Background(), mbGuardInput([]int64{101}))

	require.Error(t, err)
	require.True(t, errors.Is(err, sentinel))
	require.Empty(t, resRepo.upserted, "no cost row may be written when MB membership is unknown")
}
