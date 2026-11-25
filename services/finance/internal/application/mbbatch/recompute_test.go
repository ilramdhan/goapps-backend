package mbbatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// DAG: A <- B <- C (B consumes A, C consumes B); D independent; E consumes D.
var recomputeHeads = []MBHeadCandidate{
	{MBHID: "A", Code: "MB-A", CostProductID: 1},
	{MBHID: "B", Code: "MB-B", CostProductID: 2},
	{MBHID: "C", Code: "MB-C", CostProductID: 3},
	{MBHID: "D", Code: "MB-D", CostProductID: 4},
	{MBHID: "E", Code: "MB-E", CostProductID: 5},
}

var recomputeEdges = []MBEdge{
	{MBHID: "B", RefMBHID: "A"},
	{MBHID: "C", RefMBHID: "B"},
	{MBHID: "E", RefMBHID: "D"},
}

func ids(cs []MBHeadCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.MBHID
	}
	return out
}

func TestPlanRecompute_TransitiveDependentsInTopoOrder(t *testing.T) {
	t.Parallel()
	plan := planRecompute(recomputeHeads, recomputeEdges, map[string][]int64{
		"202609": {1},    // A -> A, B, C
		"202610": {2, 4}, // B, D -> B, C, D, E
	})
	require.Len(t, plan, 2)
	assert.Equal(t, []string{"A", "B", "C"}, ids(plan["202609"]))
	assert.Equal(t, []string{"B", "C", "D", "E"}, ids(plan["202610"]))
}

func TestPlanRecompute_UnknownProductYieldsNoPeriod(t *testing.T) {
	t.Parallel()
	plan := planRecompute(recomputeHeads, recomputeEdges, map[string][]int64{"202609": {99}})
	assert.Empty(t, plan)
}

func TestRootProducts_IgnoresHeadsNoLongerValidated(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []int64{1, 4}, rootProducts(recomputeHeads, []string{"A", "D", "GONE"}))
}

// --- RecomputeRevalidated period selection (no DB: job Create fails, so the run stops
// right after the period was selected; each Create records its period). ---

type periodJobRepo struct {
	costcalcdom.JobRepository
	periods []string
}

func (r *periodJobRepo) Create(_ context.Context, j *costcalcdom.Job) error {
	r.periods = append(r.periods, j.Period())
	return errCreateReached
}

type fakePeriods struct {
	got    []int64
	active map[string][]int64
	err    error
}

func (f *fakePeriods) ListActiveSnapshotPeriods(_ context.Context, pids []int64) (map[string][]int64, error) {
	f.got = pids
	return f.active, f.err
}

func newRecomputeHandler(jobs costcalcdom.JobRepository, lock costcalcdom.PeriodLockChecker, periods SnapshotPeriodReader) *TriggerHandler {
	svc := NewService(nil, &fakeHeadReader{heads: recomputeHeads}, &fakeEdgeReader{edges: recomputeEdges}, nil, nil, nil, nil, nil)
	return NewTriggerHandler(svc, jobs, WithPeriodLock(lock), WithSnapshotPeriods(periods))
}

func TestRecomputeRevalidated_OneJobPerUnlockedPeriod_LockedSkipped(t *testing.T) {
	t.Parallel()
	jobs := &periodJobRepo{}
	periods := &fakePeriods{active: map[string][]int64{"202608": {1}, "202609": {1}, "202610": {1}}}
	lock := &stubPeriodLock{locked: map[string]bool{"202608/ACTUAL": true}}
	h := newRecomputeHandler(jobs, lock, periods)

	h.RecomputeRevalidated(context.Background(), []string{"A"}, "tester")

	assert.Equal(t, []int64{1}, periods.got, "only the re-validated head's product is probed for periods")
	assert.Equal(t, []string{"202609", "202610"}, jobs.periods, "locked 202608 skipped, one cal_job per other period")
}

func TestRecomputeRevalidated_NoActiveSnapshot_NoJob(t *testing.T) {
	t.Parallel()
	jobs := &periodJobRepo{}
	h := newRecomputeHandler(jobs, nil, &fakePeriods{active: map[string][]int64{}})
	h.RecomputeRevalidated(context.Background(), []string{"A"}, "tester")
	assert.Empty(t, jobs.periods)
}

func TestRecomputeRevalidated_FailuresAreSwallowed(t *testing.T) {
	t.Parallel()
	jobs := &periodJobRepo{}
	h := newRecomputeHandler(jobs, &stubPeriodLock{err: errors.New("db down")},
		&fakePeriods{active: map[string][]int64{"202609": {1}}})
	require.NotPanics(t, func() { h.RecomputeRevalidated(context.Background(), []string{"A"}, "u") })
	assert.Empty(t, jobs.periods, "lock-check error skips the period")

	h = newRecomputeHandler(jobs, nil, &fakePeriods{err: errors.New("db down")})
	require.NotPanics(t, func() { h.RecomputeRevalidated(context.Background(), []string{"A"}, "u") })
	assert.Empty(t, jobs.periods)
}

func TestRecomputeRevalidated_WithoutPeriodReader_IsNoOp(t *testing.T) {
	t.Parallel()
	jobs := &periodJobRepo{}
	h := NewTriggerHandler(nil, jobs)
	h.RecomputeRevalidated(context.Background(), []string{"A"}, "u")
	h.TriggerRevalidateRecompute(context.Background(), []string{"A"}, "u")
	assert.Empty(t, jobs.periods)
}
