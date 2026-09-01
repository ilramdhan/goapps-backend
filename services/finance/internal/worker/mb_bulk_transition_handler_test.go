package worker

// Internal test package: exercises MBBulkTransitionHandler.Handle end-to-end
// against real appmbhead.ForceUnvalidateHandler/SubmitHandler/ValidateHandler
// instances (their fields are concrete struct pointers, not interfaces, so they
// cannot be swapped for a mock at the worker's boundary) backed by local fakes for
// mbhead.Repository and mbparam.Repository, plus the parent/child
// batch-completion + notify-once property, mirroring
// costsheet_export_batch_test.go's shape.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbheadbulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// --- fakes ------------------------------------------------------------------

// fakeWorkerMBHeadRepo is a minimal, hand-rolled mbhead.Repository: only GetByID,
// Transition, TransitionWithAutoGen and ForceUnvalidateTransition are exercised by
// MBBulkTransitionHandler's dispatch, so everything else panics if called —
// mirroring fakeFrozenMBHeadRepo's "record calls, in-memory store" shape rather
// than a testify mock, since these dispatch tests just need call-counting, not
// matcher assertions.
type fakeWorkerMBHeadRepo struct {
	byID map[uuid.UUID]*mbheaddomain.Entity

	transitionCalls               int
	transitionWithAutoGenCalls    int
	forceUnvalidateTransitionCall int

	transitionErr                error
	transitionWithAutoGenErr     error
	forceUnvalidateTransitionErr error
}

func newFakeWorkerMBHeadRepo() *fakeWorkerMBHeadRepo {
	return &fakeWorkerMBHeadRepo{byID: map[uuid.UUID]*mbheaddomain.Entity{}}
}

func (f *fakeWorkerMBHeadRepo) put(e *mbheaddomain.Entity) { f.byID[e.ID()] = e }

func (f *fakeWorkerMBHeadRepo) Create(_ context.Context, _ *mbheaddomain.Entity) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) GetByID(_ context.Context, id uuid.UUID) (*mbheaddomain.Entity, error) {
	e, ok := f.byID[id]
	if !ok {
		return nil, mbheaddomain.ErrNotFound
	}
	return e, nil
}

func (f *fakeWorkerMBHeadRepo) GetByMBCosting(_ context.Context, _ string) (*mbheaddomain.Entity, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) List(_ context.Context, _ mbheaddomain.ListFilter) ([]*mbheaddomain.Entity, int64, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) Update(_ context.Context, _ *mbheaddomain.Entity) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ExistsByMBCosting(_ context.Context, _ string) (bool, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ExistsByID(_ context.Context, _ uuid.UUID) (bool, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ListAll(_ context.Context, _ mbheaddomain.ExportFilter) ([]*mbheaddomain.Entity, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) Transition(_ context.Context, _ uuid.UUID, _, _ string, _ int32, _, _ string, _ *mbheaddomain.ParamSnapshot) error {
	f.transitionCalls++
	return f.transitionErr
}

func (f *fakeWorkerMBHeadRepo) TransitionWithAutoGen(_ context.Context, _ uuid.UUID, _, _ string, _ int32, _, _ string, _ *mbheaddomain.ParamSnapshot, _ *mbheaddomain.Entity) error {
	f.transitionWithAutoGenCalls++
	return f.transitionWithAutoGenErr
}

func (f *fakeWorkerMBHeadRepo) ListShades(_ context.Context, _ uuid.UUID) ([]mbheaddomain.Shade, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ReplaceShades(_ context.Context, _ uuid.UUID, _ []mbheaddomain.Shade, _ string) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ExistsByVSNumber(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ExistsByDevCode(_ context.Context, _ string, _ uuid.UUID) (bool, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) RefreezeCostParams(_ context.Context, _ uuid.UUID, _ *mbheaddomain.Entity, _ *mbheaddomain.ParamSnapshot) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerMBHeadRepo) ForceUnvalidateTransition(_ context.Context, _ uuid.UUID, _ int, _, _ string) error {
	f.forceUnvalidateTransitionCall++
	return f.forceUnvalidateTransitionErr
}

// fakeWorkerParamRepo is a minimal mbparam.Repository — only ListActive is called
// by ValidateHandler.resolveParamSnapshot.
type fakeWorkerParamRepo struct {
	active []*mbparam.Entity
	err    error
}

func (f *fakeWorkerParamRepo) Create(_ context.Context, _ *mbparam.Entity) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) Update(_ context.Context, _ *mbparam.Entity) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) Delete(_ context.Context, _ string) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) GetByID(_ context.Context, _ string) (*mbparam.Entity, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) List(_ context.Context, _ mbparam.ListFilter) ([]*mbparam.Entity, int64, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) ListActive(_ context.Context) ([]*mbparam.Entity, error) {
	return f.active, f.err
}

func (f *fakeWorkerParamRepo) ListAll(_ context.Context, _ mbparam.ExportFilter) ([]*mbparam.Entity, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) GetByCode(_ context.Context, _ string) (*mbparam.Entity, error) {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) CreateOption(_ context.Context, _ *mbparam.Option) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) UpdateOption(_ context.Context, _ *mbparam.Option) error {
	panic("not used by MBBulkTransitionHandler tests")
}

func (f *fakeWorkerParamRepo) DeleteOption(_ context.Context, _ string) error {
	panic("not used by MBBulkTransitionHandler tests")
}

// workerJobRepoMock is a testify mock for job.Repository, mirroring
// batchJobRepoMock in costsheet_export_batch_test.go exactly.
type workerJobRepoMock struct{ mock.Mock }

func (m *workerJobRepoMock) Create(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *workerJobRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*job.Execution, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *workerJobRepoMock) GetByCode(ctx context.Context, code string) (*job.Execution, error) {
	args := m.Called(ctx, code)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *workerJobRepoMock) List(ctx context.Context, f job.ListFilter) ([]*job.Execution, int64, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*job.Execution), args.Get(1).(int64), args.Error(2)
}

func (m *workerJobRepoMock) UpdateStatus(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *workerJobRepoMock) UpdateProgress(ctx context.Context, id uuid.UUID, p int) error {
	return m.Called(ctx, id, p).Error(0)
}

func (m *workerJobRepoMock) AddLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *workerJobRepoMock) UpdateLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *workerJobRepoMock) HasActiveJob(ctx context.Context, t job.Type, p string) (bool, error) {
	args := m.Called(ctx, t, p)
	return args.Bool(0), args.Error(1)
}

func (m *workerJobRepoMock) GetNextSequence(ctx context.Context, t job.Type, p string) (int, error) {
	args := m.Called(ctx, t, p)
	return args.Int(0), args.Error(1)
}

func (m *workerJobRepoMock) CreateChildren(ctx context.Context, execs []*job.Execution) error {
	return m.Called(ctx, execs).Error(0)
}

func (m *workerJobRepoMock) IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	args := m.Called(ctx, parentJobID, success)
	return args.Bool(0), args.Error(1)
}

func (m *workerJobRepoMock) ListChildren(ctx context.Context, parentJobID uuid.UUID) ([]*job.Execution, error) {
	args := m.Called(ctx, parentJobID)
	var out []*job.Execution
	if v := args.Get(0); v != nil {
		out = v.([]*job.Execution)
	}
	return out, args.Error(1)
}

// --- fixtures -----------------------------------------------------------------

// headInState builds a minimal MB Head fixture in the given entry status, mirroring
// application/mbhead's own headInState test helper (reject_return_to_draft_handler_test.go)
// — duplicated here rather than imported since that helper lives in an internal
// _test.go file of a different package.
func headInState(entryStatus string) *mbheaddomain.Entity {
	return mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		entryStatus, false, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "34", "S",
		nil,
	)
}

func activeParams() []*mbparam.Entity {
	codes := []struct{ code, typ string }{
		{"WASTE", "numeric"},
		{"QUALITY_LOSS", "numeric"},
		{"EFFICIENCY", "numeric"},
		{"DEV_EXPENSE", "numeric"},
		{"PACKING", "numeric"},
		{"MB_PROD_PER_DAY", "numeric"},
		{"THROUGHPUT_PER_HOUR", "picklist"},
		{"NO_OF_PROCESS", "picklist"},
	}
	out := make([]*mbparam.Entity, 0, len(codes))
	for _, c := range codes {
		out = append(out, mbparam.Reconstruct(
			uuid.NewString(), c.code, c.code, "", c.typ, "1", "1", "", 0, true,
			time.Now().Format(time.RFC3339), "seed", "", "", "", "",
		))
	}
	return out
}

func newChildExecWorker(t *testing.T, subtype string, parentID uuid.UUID) *job.Execution {
	t.Helper()
	exec, err := job.NewChildExecution(job.TypeMBBulkTransition, subtype, "", "admin", 5, nil, parentID)
	require.NoError(t, err)
	require.NoError(t, exec.Start())
	return exec
}

func newParentExecWorker(t *testing.T, id uuid.UUID, total, completed, failed int) *job.Execution {
	t.Helper()
	code, err := job.NewCode("MBBULK-1")
	require.NoError(t, err)
	return job.Reconstitute(
		id, code, job.TypeMBBulkTransition, mbheadbulk.ActionSubmit,
		"", job.StatusProcessing, 5,
		nil, nil, "",
		0, 0, 3,
		time.Now(), nil, nil,
		"admin", "", nil,
		nil,
		nil, total, completed, failed,
	)
}

// --- dispatch tests -------------------------------------------------------------

// TestMBBulkTransitionHandler_Handle_DispatchesPerSubtype proves each Subtype
// routes to the matching downstream application handler and no other — evidenced
// by each fake's own call counter, since the three concrete handlers cannot be
// substituted with a single spy.
func TestMBBulkTransitionHandler_Handle_DispatchesPerSubtype(t *testing.T) {
	tests := []struct {
		name         string
		subtype      string
		fromState    string
		wantForceUn  int
		wantSubmit   int
		wantValidate int
	}{
		{"force_unvalidate calls ForceUnvalidateHandler only", mbheadbulk.ActionForceUnvalidate, mbheaddomain.StatusValidated, 1, 0, 0},
		{"submit calls SubmitHandler only", mbheadbulk.ActionSubmit, mbheaddomain.StatusDraft, 0, 1, 0},
		{"validate calls ValidateHandler only", mbheadbulk.ActionValidate, mbheaddomain.StatusSubmitted, 0, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headRepo := newFakeWorkerMBHeadRepo()
			head := headInState(tt.fromState)
			headRepo.put(head)
			paramRepo := &fakeWorkerParamRepo{active: activeParams()}

			forceUnvalidateH := appmbhead.NewForceUnvalidateHandler(headRepo)
			submitH := appmbhead.NewSubmitHandler(headRepo)
			validateH := appmbhead.NewValidateHandler(headRepo, paramRepo)

			jobRepo := &workerJobRepoMock{}
			parentID := uuid.New()
			child := newChildExecWorker(t, tt.subtype, parentID)

			jobRepo.On("GetByID", mock.Anything, child.ID()).Return(child, nil).Once()
			jobRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil)
			jobRepo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(false, nil).Once()

			h := NewMBBulkTransitionHandler(jobRepo, forceUnvalidateH, submitH, validateH, zerolog.Nop())
			msg := rabbitmq.JobMessage{
				JobID: child.ID().String(), JobType: string(job.TypeMBBulkTransition),
				Subtype: tt.subtype, MbhID: head.ID().String(), CreatedBy: "admin",
			}

			err := h.Handle(context.Background(), msg)
			require.NoError(t, err, "Handle never returns an error itself — failures are recorded on the job")

			require.Equal(t, tt.wantForceUn, headRepo.forceUnvalidateTransitionCall)
			require.Equal(t, tt.wantSubmit, headRepo.transitionCalls)
			require.Equal(t, tt.wantValidate, headRepo.transitionWithAutoGenCalls)

			jobRepo.AssertExpectations(t)
		})
	}
}

// TestMBBulkTransitionHandler_Handle_DownstreamError_MarksChildFailed proves a
// downstream handler error (state-machine gate rejection here) is recorded as a
// FAILED child and reported to the parent as a failure, not silently dropped or
// surfaced as a transport error from Handle.
func TestMBBulkTransitionHandler_Handle_DownstreamError_MarksChildFailed(t *testing.T) {
	headRepo := newFakeWorkerMBHeadRepo()
	// DRAFT cannot be force-unvalidated (only VALIDATED qualifies) — Entity.ForceUnvalidate
	// returns ErrInvalidTransition, so this deterministically fails without touching the DB.
	head := headInState(mbheaddomain.StatusDraft)
	headRepo.put(head)

	forceUnvalidateH := appmbhead.NewForceUnvalidateHandler(headRepo)
	submitH := appmbhead.NewSubmitHandler(headRepo)
	validateH := appmbhead.NewValidateHandler(headRepo, &fakeWorkerParamRepo{})

	jobRepo := &workerJobRepoMock{}
	parentID := uuid.New()
	child := newChildExecWorker(t, mbheadbulk.ActionForceUnvalidate, parentID)

	jobRepo.On("GetByID", mock.Anything, child.ID()).Return(child, nil).Once()
	// Handle persists PROCESSING first (exec.Start() is a no-op warn here since the
	// fixture is already Started, but the interim UpdateStatus call still happens),
	// then persists the terminal FAILED status once the downstream gate rejects.
	jobRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.ID() == child.ID() && e.Status() == job.StatusFailed
	})).Return(nil).Once()
	jobRepo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(false, nil).Once()

	h := NewMBBulkTransitionHandler(jobRepo, forceUnvalidateH, submitH, validateH, zerolog.Nop())
	msg := rabbitmq.JobMessage{
		JobID: child.ID().String(), Subtype: mbheadbulk.ActionForceUnvalidate,
		MbhID: head.ID().String(), CreatedBy: "admin",
	}

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, 0, headRepo.forceUnvalidateTransitionCall, "the domain gate must reject before the repo is ever called")

	jobRepo.AssertExpectations(t)
}

// --- child-completion / notify-once tests ----------------------------------------

// TestMBBulkTransitionHandler_ChildCompletion_TerminalStatusOnce mirrors
// TestCostSheetExportHandler_ChildCompletion_NotifiesExactlyOnceWhenBatchDone's
// shape for MBBulkTransitionHandler.handleChildCompletion: a child completing when
// the batch is not yet done must not touch the parent job row at all (proving no
// premature/double terminal-status write); the child that completes the batch must
// mark the parent COMPLETE (or FAIL, if every child failed) EXACTLY ONCE.
func TestMBBulkTransitionHandler_ChildCompletion_TerminalStatusOnce(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()

	t.Run("batch not yet complete: parent job row untouched", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}
		jobRepo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(false, nil).Once()

		h := NewMBBulkTransitionHandler(jobRepo, nil, nil, nil, zerolog.Nop())
		child := newChildExecWorker(t, mbheadbulk.ActionSubmit, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), Subtype: mbheadbulk.ActionSubmit, MbhID: uuid.NewString(), CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), child, msg, true)

		jobRepo.AssertExpectations(t)
		jobRepo.AssertNotCalled(t, "GetByID", mock.Anything, parentID)
	})

	t.Run("batch complete: parent marked COMPLETE exactly once", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}
		parent := newParentExecWorker(t, parentID, 3, 2, 0)

		jobRepo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(true, nil).Once()
		jobRepo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
		jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
			return e.ID() == parentID && e.Status() == job.StatusSuccess
		})).Return(nil).Once()

		h := NewMBBulkTransitionHandler(jobRepo, nil, nil, nil, zerolog.Nop())
		child := newChildExecWorker(t, mbheadbulk.ActionSubmit, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), Subtype: mbheadbulk.ActionSubmit, MbhID: uuid.NewString(), CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), child, msg, true)

		jobRepo.AssertExpectations(t)
		jobRepo.AssertNumberOfCalls(t, "UpdateStatus", 1)
	})

	t.Run("batch complete with all children failed: parent marked FAILED exactly once", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}
		parent := newParentExecWorker(t, parentID, 3, 0, 3)

		jobRepo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(true, nil).Once()
		jobRepo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
		jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
			return e.ID() == parentID && e.Status() == job.StatusFailed
		})).Return(nil).Once()

		h := NewMBBulkTransitionHandler(jobRepo, nil, nil, nil, zerolog.Nop())
		child := newChildExecWorker(t, mbheadbulk.ActionSubmit, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), Subtype: mbheadbulk.ActionSubmit, MbhID: uuid.NewString(), CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), child, msg, false)

		jobRepo.AssertExpectations(t)
		jobRepo.AssertNumberOfCalls(t, "UpdateStatus", 1)
	})

	t.Run("non-child job (nil parent id): no-op, no double-fire", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}

		h := NewMBBulkTransitionHandler(jobRepo, nil, nil, nil, zerolog.Nop())
		standalone, err := job.NewExecution(job.TypeMBBulkTransition, mbheadbulk.ActionSubmit, "", "admin", 5, nil)
		require.NoError(t, err)
		msg := rabbitmq.JobMessage{JobID: standalone.ID().String(), Subtype: mbheadbulk.ActionSubmit, MbhID: uuid.NewString(), CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), standalone, msg, true)

		jobRepo.AssertNotCalled(t, "IncrementChildProgress", mock.Anything, mock.Anything, mock.Anything)
		jobRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
	})
}
