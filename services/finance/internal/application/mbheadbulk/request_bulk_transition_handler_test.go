package mbheadbulk_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbheadbulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// jobRepoMock is a small testify mock for job.Repository — only the methods
// exercised by RequestBulkTransitionHandler are stubbed, mirroring
// costsheet's jobRepoMock (see application/costsheet/mocks_test.go).
type jobRepoMock struct{ mock.Mock }

func (m *jobRepoMock) Create(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *jobRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*job.Execution, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *jobRepoMock) GetByCode(ctx context.Context, code string) (*job.Execution, error) {
	args := m.Called(ctx, code)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *jobRepoMock) List(ctx context.Context, f job.ListFilter) ([]*job.Execution, int64, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*job.Execution), args.Get(1).(int64), args.Error(2)
}

func (m *jobRepoMock) UpdateStatus(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *jobRepoMock) UpdateProgress(ctx context.Context, id uuid.UUID, p int) error {
	return m.Called(ctx, id, p).Error(0)
}

func (m *jobRepoMock) AddLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *jobRepoMock) UpdateLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *jobRepoMock) HasActiveJob(ctx context.Context, t job.Type, p string) (bool, error) {
	args := m.Called(ctx, t, p)
	return args.Bool(0), args.Error(1)
}

func (m *jobRepoMock) GetNextSequence(ctx context.Context, t job.Type, p string) (int, error) {
	args := m.Called(ctx, t, p)
	return args.Int(0), args.Error(1)
}

func (m *jobRepoMock) CreateChildren(ctx context.Context, execs []*job.Execution) error {
	return m.Called(ctx, execs).Error(0)
}

func (m *jobRepoMock) IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	args := m.Called(ctx, parentJobID, success)
	return args.Bool(0), args.Error(1)
}

func (m *jobRepoMock) ListChildren(ctx context.Context, parentJobID uuid.UUID) ([]*job.Execution, error) {
	args := m.Called(ctx, parentJobID)
	var out []*job.Execution
	if v := args.Get(0); v != nil {
		out = v.([]*job.Execution)
	}
	return out, args.Error(1)
}

// publisherMock is a testify mock for mbheadbulk.BulkTransitionJobPublisher.
// failFor, when non-nil, marks a subset of mbhIDs whose publish call must
// fail — everything else succeeds. This lets tests exercise the "publish
// failure fails only that one child" contract without hand-rolling a fake
// per test.
type publisherMock struct {
	mock.Mock
	failFor map[string]bool
}

func (m *publisherMock) PublishMBBulkTransition(ctx context.Context, jobID, mbhID, action, reason, createdBy string) error {
	m.Called(ctx, jobID, mbhID, action, reason, createdBy)
	if m.failFor != nil && m.failFor[mbhID] {
		return errors.New("publish failed for " + mbhID)
	}
	return nil
}

func TestRequestBulkTransitionHandler_Validate(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}

	t.Run("publisher nil", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, nil, nil)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			MBHIDs: []string{"a"}, Action: mbheadbulk.ActionSubmit, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, mbheadbulk.ErrPublisherUnavailable)
	})

	t.Run("empty mbh_ids", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			Action: mbheadbulk.ActionSubmit, CreatedBy: "admin",
		})
		require.Error(t, err)
	})

	t.Run("unknown action", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			MBHIDs: []string{"a"}, Action: "bogus", CreatedBy: "admin",
		})
		require.Error(t, err)
	})

	t.Run("missing created by", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, nil)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			MBHIDs: []string{"a"}, Action: mbheadbulk.ActionSubmit,
		})
		require.Error(t, err)
	})

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestRequestBulkTransitionHandler_OneChildPerMBHID_NoChunking proves the
// parent job's total_children matches len(MBHIDs) exactly (no chunking, per
// the package doc comment) and exactly one child is created per mbh_id.
func TestRequestBulkTransitionHandler_OneChildPerMBHID_NoChunking(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}

	mbhIDs := []string{"mbh-1", "mbh-2", "mbh-3", "mbh-4", "mbh-5"}

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == len(mbhIDs)
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.MatchedBy(func(execs []*job.Execution) bool {
		return len(execs) == len(mbhIDs)
	})).Return(nil).Once()
	for _, id := range mbhIDs {
		pub.On("PublishMBBulkTransition", mock.Anything, mock.Anything, id, mbheadbulk.ActionForceUnvalidate, "bulk regen", "admin").
			Return(nil).Once()
	}

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, nil)
	res, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs:    mbhIDs,
		Action:    mbheadbulk.ActionForceUnvalidate,
		Reason:    "bulk regen",
		CreatedBy: "admin",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Execution.IsParent())
	assert.Equal(t, len(mbhIDs), res.Execution.TotalChildren())

	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestRequestBulkTransitionHandler_PerChildPublishFailure_DoesNotAbortBatch
// proves a publish failure on a SUBSET of children is recorded on those
// children only (failJob + IncrementChildProgress(success=false)) — it must
// not stop the parent job or the other children from being created, and the
// other children's publish calls must still happen. Critically, per-child
// publish failures are a normal, expected, partially-successful outcome: they
// must NOT surface as a non-nil error from Handle, since the parent job and
// every child (including the ones that DID publish) are already durably
// persisted — the caller needs the job info back, not a failure.
func TestRequestBulkTransitionHandler_PerChildPublishFailure_DoesNotAbortBatch(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	mbhIDs := []string{"mbh-ok-1", "mbh-fail-1", "mbh-ok-2", "mbh-fail-2"}
	pub := &publisherMock{failFor: map[string]bool{"mbh-fail-1": true, "mbh-fail-2": true}}

	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.IsParent() && e.TotalChildren() == len(mbhIDs)
	})).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.MatchedBy(func(execs []*job.Execution) bool {
		return len(execs) == len(mbhIDs)
	})).Return(nil).Once()
	for _, id := range mbhIDs {
		pub.On("PublishMBBulkTransition", mock.Anything, mock.Anything, id, mbheadbulk.ActionSubmit, "", "admin").
			Return(nil).Once()
	}
	// Two children fail to publish: each gets marked failed and the parent's
	// failed-children counter incremented — never aborting the loop.
	repo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Twice()
	repo.On("IncrementChildProgress", mock.Anything, mock.Anything, false).Return(false, nil).Twice()
	// Handle refreshes the parent from the repository afterward to report
	// accurate counters. IncrementChildProgress above only updates counters at the
	// repository level (this mock does not mutate the in-memory execution passed
	// to Create), so simulate that refresh returning a freshly reconstituted
	// parent whose failed-children counter reflects the two recorded failures —
	// its identity need not match the original in-memory parent, only its
	// counters matter for this assertion.
	refreshedParent, err := job.NewParentExecution(job.TypeMBBulkTransition, mbheadbulk.ActionSubmit, "", "admin", 5, nil, len(mbhIDs))
	require.NoError(t, err)
	refreshedParent.IncrementFailedChildren()
	refreshedParent.IncrementFailedChildren()
	repo.On("GetByID", mock.Anything, mock.Anything).Return(refreshedParent, nil).Once()

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, nil)
	res, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs:    mbhIDs,
		Action:    mbheadbulk.ActionSubmit,
		CreatedBy: "admin",
	})

	// The two publish failures are recorded on their own children (and on the
	// parent's counters) but do NOT fail Handle: Create and CreateChildren above
	// were still called exactly once each (parent + all children persisted), and
	// PublishMBBulkTransition was still attempted for every mbh_id, including the
	// ones after each failure — proving one child's publish failure never aborts
	// the batch nor Handle itself.
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Execution)
	assert.Equal(t, 2, res.Execution.FailedChildren())
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// compositionLookupStub is a minimal stand-in for mbheadbulk.CompositionRefLookup.
type compositionLookupStub struct {
	edges []mbcomposition.BatchRefEdge
	err   error
}

func (s *compositionLookupStub) ListMBRefEdgesForBatch(_ context.Context, _ []string) ([]mbcomposition.BatchRefEdge, error) {
	return s.edges, s.err
}

// publishOrder extracts the mbhID argument (third positional arg to
// PublishMBBulkTransition, after ctx and jobID) from each recorded call, in the
// order Handle actually invoked them — the order children were published in.
func publishOrder(pub *publisherMock) []string {
	order := make([]string, 0, len(pub.Calls))
	for _, call := range pub.Calls {
		if call.Method != "PublishMBBulkTransition" {
			continue
		}
		order = append(order, call.Arguments.String(2))
	}
	return order
}

// setupOrderingExpectations wires the Create/CreateChildren/Publish mock
// expectations shared by the ordering tests below — only the publish order
// varies per test, everything else about a successful Handle call is the same.
func setupOrderingExpectations(repo *jobRepoMock, pub *publisherMock, mbhIDs []string, action string) {
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	repo.On("CreateChildren", mock.Anything, mock.Anything).Return(nil).Once()
	for _, id := range mbhIDs {
		pub.On("PublishMBBulkTransition", mock.Anything, mock.Anything, id, action, "", "admin").Return(nil).Once()
	}
}

// TestRequestBulkTransitionHandler_OrderByDependency_NoDependencies_OrderUnchanged
// proves a batch with no within-batch composition references publishes children in
// the exact order they were submitted in.
func TestRequestBulkTransitionHandler_OrderByDependency_NoDependencies_OrderUnchanged(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}
	comp := &compositionLookupStub{}
	mbhIDs := []string{"mbh-1", "mbh-2", "mbh-3"}
	setupOrderingExpectations(repo, pub, mbhIDs, mbheadbulk.ActionValidate)

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, comp)
	_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs: mbhIDs, Action: mbheadbulk.ActionValidate, CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, mbhIDs, publishOrder(pub))
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestRequestBulkTransitionHandler_OrderByDependency_LinearChain_DependencyFirst
// proves that when A's recipe references B (both in the same batch), B is
// published before A even though the request listed A first — the fix for the
// production bug where A's cost generation could hit B's still-NULL
// mbh_cost_product_id.
func TestRequestBulkTransitionHandler_OrderByDependency_LinearChain_DependencyFirst(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}
	comp := &compositionLookupStub{edges: []mbcomposition.BatchRefEdge{
		{MbhID: "mbh-a", RefMbhID: "mbh-b"}, // A depends on B
	}}
	mbhIDs := []string{"mbh-a", "mbh-b"}
	setupOrderingExpectations(repo, pub, mbhIDs, mbheadbulk.ActionValidate)

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, comp)
	_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs: mbhIDs, Action: mbheadbulk.ActionValidate, CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"mbh-b", "mbh-a"}, publishOrder(pub))
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestRequestBulkTransitionHandler_OrderByDependency_Cycle_DoesNotHangAndFallsBack
// proves a within-batch reference cycle (A depends on B, B depends on A) neither
// hangs nor crashes Handle, and produces a valid publish order: the uninvolved
// third head (no dependency edges at all) is free to be placed as soon as it's
// ready — which Kahn's algorithm does immediately, ahead of the stalled cyclic
// pair — while A and B, unable to ever become "ready" of each other, fall back to
// their original relative order (A before B) once the stall is detected.
func TestRequestBulkTransitionHandler_OrderByDependency_Cycle_DoesNotHangAndFallsBack(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}
	comp := &compositionLookupStub{edges: []mbcomposition.BatchRefEdge{
		{MbhID: "mbh-a", RefMbhID: "mbh-b"},
		{MbhID: "mbh-b", RefMbhID: "mbh-a"},
	}}
	mbhIDs := []string{"mbh-a", "mbh-b", "mbh-c"}
	setupOrderingExpectations(repo, pub, mbhIDs, mbheadbulk.ActionValidate)

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, comp)
	res, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs: mbhIDs, Action: mbheadbulk.ActionValidate, CreatedBy: "admin",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	order := publishOrder(pub)
	assert.ElementsMatch(t, mbhIDs, order, "cycle handling must not drop or duplicate any child")
	assert.Equal(t, []string{"mbh-c", "mbh-a", "mbh-b"}, order,
		"mbh-c has no dependency edges so it is published as soon as it's ready; "+
			"mbh-a/mbh-b are mutually cyclic so they fall back to their original relative order")
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestRequestBulkTransitionHandler_OrderByDependency_LookupError_FallsBackToOriginalOrder
// proves a composition-lookup failure is swallowed (best-effort) rather than
// failing the whole bulk request — children still publish in original order.
func TestRequestBulkTransitionHandler_OrderByDependency_LookupError_FallsBackToOriginalOrder(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &publisherMock{}
	comp := &compositionLookupStub{err: errors.New("db unavailable")}
	mbhIDs := []string{"mbh-1", "mbh-2"}
	setupOrderingExpectations(repo, pub, mbhIDs, mbheadbulk.ActionSubmit)

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub, comp)
	_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs: mbhIDs, Action: mbheadbulk.ActionSubmit, CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, mbhIDs, publishOrder(pub))
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}
