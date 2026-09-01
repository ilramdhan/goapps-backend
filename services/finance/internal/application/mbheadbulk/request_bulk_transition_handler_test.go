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
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, nil)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			MBHIDs: []string{"a"}, Action: mbheadbulk.ActionSubmit, CreatedBy: "admin",
		})
		require.ErrorIs(t, err, mbheadbulk.ErrPublisherUnavailable)
	})

	t.Run("empty mbh_ids", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			Action: mbheadbulk.ActionSubmit, CreatedBy: "admin",
		})
		require.Error(t, err)
	})

	t.Run("unknown action", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub)
		_, err := h.Handle(context.Background(), mbheadbulk.RequestBulkTransitionCommand{
			MBHIDs: []string{"a"}, Action: "bogus", CreatedBy: "admin",
		})
		require.Error(t, err)
	})

	t.Run("missing created by", func(t *testing.T) {
		h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub)
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

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub)
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

	h := mbheadbulk.NewRequestBulkTransitionHandler(repo, pub)
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
