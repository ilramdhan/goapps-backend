package worker

// Internal test package: exercises ProductParamBulkHandler.Handle end-to-end
// against a fake cpp.Repository (ApplyBulkOperations only — everything else
// panics if called, since no other method is exercised by this handler) and
// workerJobRepoMock (the testify mock for job.Repository already defined in
// mb_bulk_transition_handler_test.go, reused here directly since both files
// live in package worker), mirroring that file's shape.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/productparambulk"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// fakeCPPRepo is a minimal, hand-rolled cpp.Repository: only
// ApplyBulkOperations is exercised by ProductParamBulkHandler, so every
// other method panics if called.
type fakeCPPRepo struct {
	applyCalls int
	gotOps     []cpp.BulkOp
	gotSkip    bool

	outcomes []cpp.BulkOpOutcome
	err      error
}

func (f *fakeCPPRepo) ApplyBulkOperations(_ context.Context, _ int64, ops []cpp.BulkOp, _ string, skipMissingApplicable bool) ([]cpp.BulkOpOutcome, error) {
	f.applyCalls++
	f.gotOps = ops
	f.gotSkip = skipMissingApplicable
	return f.outcomes, f.err
}

func (f *fakeCPPRepo) ListForProduct(_ context.Context, _ int64, _ bool) ([]cpp.RequiredEntry, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) GetMeta(_ context.Context, _ uuid.UUID) (*cpp.ParamMeta, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) ProductExists(_ context.Context, _ int64) (bool, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) IsProductLocked(_ context.Context, _ int64) (bool, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) Upsert(_ context.Context, _ *cpp.Value) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) Delete(_ context.Context, _ int64, _ uuid.UUID) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) MissingRequired(_ context.Context, _ int64) ([]cpp.ParamMeta, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) AddApplicable(_ context.Context, _ *cpp.Applicability) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) RemoveApplicable(_ context.Context, _ int64, _ uuid.UUID) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) UpdateApplicable(_ context.Context, _ int64, _ uuid.UUID, _ *bool, _ *int32, _ string) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) ListAvailableParams(_ context.Context, _ int64) ([]cpp.ParamMeta, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) CountApplicableForProducts(_ context.Context, _ []int64) (int32, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) GetParamIDByCode(_ context.Context, _ string) (uuid.UUID, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) GetProductSysIDByCode(_ context.Context, _ string) (int64, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) ListApplicable(_ context.Context, _ int64) ([]cpp.CAPPRow, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) ListAllApplicable(_ context.Context) ([]cpp.CAPPRow, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) ListAllValues(_ context.Context) ([]cpp.CPPRow, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) GetParamCodeByID(_ context.Context, _ uuid.UUID) (string, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) GetCurrentValueAsText(_ context.Context, _ int64, _ uuid.UUID) (string, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) AddApplicableWithChildren(_ context.Context, _ int64, _ uuid.UUID, _ bool, _ string, _ []uuid.UUID) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) GetRemovePreview(_ context.Context, _ int64, _ uuid.UUID) (cpp.RemovePreview, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) RemoveApplicableWithChildren(_ context.Context, _ int64, _ uuid.UUID, _ string) error {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) BulkUpsertValues(_ context.Context, _ []cpp.CPPUpsertInput, _ string) (int, int, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) BulkUpsertApplicable(_ context.Context, _ []cpp.CAPPUpsertInput, _ string) (int, int, error) {
	panic("not used by ProductParamBulkHandler tests")
}
func (f *fakeCPPRepo) ListAllParams(_ context.Context) ([]cpp.ParamMeta, error) {
	panic("not used by ProductParamBulkHandler tests")
}

// --- fixtures ---------------------------------------------------------------

func newProductParamBulkChild(t *testing.T, parentID uuid.UUID) *job.Execution {
	t.Helper()
	exec, err := job.NewChildExecution(job.TypeProductParamBulk, "", "", "admin", 5, nil, parentID)
	require.NoError(t, err)
	require.NoError(t, exec.Start())
	return exec
}

func newProductParamBulkParent(t *testing.T, id uuid.UUID, total, completed, failed int) *job.Execution {
	t.Helper()
	code, err := job.NewCode("PPBULK-1")
	require.NoError(t, err)
	return job.Reconstitute(
		id, code, job.TypeProductParamBulk, "",
		"", job.StatusProcessing, 5,
		nil, nil, "",
		0, 0, 3,
		time.Now(), nil, nil,
		"admin", "", nil,
		nil,
		nil, total, completed, failed,
	)
}

func opsJSON(t *testing.T, ops ...productparambulk.OperationDTO) string {
	t.Helper()
	b, err := json.Marshal(ops)
	require.NoError(t, err)
	return string(b)
}

// --- Handle: the one path where an error propagates ------------------------

// TestProductParamBulkHandler_Handle_UnknownJobID_ReturnsError proves the
// job-lookup failure is the ONLY path where Handle itself returns a non-nil
// error (so RabbitMQ redelivers), unlike every application-level failure
// below which is swallowed (recorded on the job, Handle returns nil).
func TestProductParamBulkHandler_Handle_UnknownJobID_ReturnsError(t *testing.T) {
	jobRepo := &workerJobRepoMock{}
	missingID := uuid.New()
	jobRepo.On("GetByID", mock.Anything, missingID).Return(nil, errors.New("not found")).Once()

	h := NewProductParamBulkHandler(jobRepo, &fakeCPPRepo{}, zerolog.Nop())
	msg := rabbitmq.JobMessage{JobID: missingID.String(), CreatedBy: "admin"}

	err := h.Handle(context.Background(), msg)
	require.Error(t, err)
	jobRepo.AssertExpectations(t)
}

// TestProductParamBulkHandler_Handle_InvalidJobID_ReturnsError proves a
// malformed JobID (not even a UUID) also fails before any repo call.
func TestProductParamBulkHandler_Handle_InvalidJobID_ReturnsError(t *testing.T) {
	jobRepo := &workerJobRepoMock{}
	h := NewProductParamBulkHandler(jobRepo, &fakeCPPRepo{}, zerolog.Nop())
	msg := rabbitmq.JobMessage{JobID: "not-a-uuid", CreatedBy: "admin"}

	err := h.Handle(context.Background(), msg)
	require.Error(t, err)
	jobRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

// --- Handle: success path ---------------------------------------------------

// TestProductParamBulkHandler_Handle_Success_MarksCompletedAndIncrementsParent
// proves the happy path: ApplyBulkOperations is invoked with the decoded ops
// and skip flag, the job transitions to COMPLETED, and the parent batch is
// notified of a success.
func TestProductParamBulkHandler_Handle_Success_MarksCompletedAndIncrementsParent(t *testing.T) {
	jobRepo := &workerJobRepoMock{}
	parentID := uuid.New()
	child := newProductParamBulkChild(t, parentID)

	paramID := uuid.NewString()
	msg := rabbitmq.JobMessage{
		JobID:                 child.ID().String(),
		ProductSysID:          101,
		Operations:            opsJSON(t, productparambulk.OperationDTO{Kind: productparambulk.OpAddApplicable, ParamID: paramID}),
		SkipMissingApplicable: true,
		CreatedBy:             "admin",
	}

	jobRepo.On("GetByID", mock.Anything, child.ID()).Return(child, nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.ID() == child.ID() && e.Status() == job.StatusSuccess
	})).Return(nil).Once()
	jobRepo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(false, nil).Once()

	repo := &fakeCPPRepo{outcomes: []cpp.BulkOpOutcome{{ParamID: uuid.MustParse(paramID), Kind: cpp.BulkOpAddApplicable}}}
	h := NewProductParamBulkHandler(jobRepo, repo, zerolog.Nop())

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err)

	require.Equal(t, 1, repo.applyCalls)
	require.Len(t, repo.gotOps, 1)
	require.Equal(t, cpp.BulkOpAddApplicable, repo.gotOps[0].Kind)
	require.True(t, repo.gotSkip)

	jobRepo.AssertExpectations(t)
}

// --- Handle: application-level failures are swallowed -----------------------

// TestProductParamBulkHandler_Handle_InvalidOperationsJSON_MarksFailed proves
// undecodable msg.Operations is treated as a per-child failure (FAILED status
// + IncrementChildProgress(success=false)), not a transport error.
func TestProductParamBulkHandler_Handle_InvalidOperationsJSON_MarksFailed(t *testing.T) {
	jobRepo := &workerJobRepoMock{}
	parentID := uuid.New()
	child := newProductParamBulkChild(t, parentID)

	msg := rabbitmq.JobMessage{
		JobID:        child.ID().String(),
		ProductSysID: 101,
		Operations:   "not-json",
		CreatedBy:    "admin",
	}

	jobRepo.On("GetByID", mock.Anything, child.ID()).Return(child, nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.ID() == child.ID() && e.Status() == job.StatusFailed
	})).Return(nil).Once()
	jobRepo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(false, nil).Once()

	repo := &fakeCPPRepo{}
	h := NewProductParamBulkHandler(jobRepo, repo, zerolog.Nop())

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err, "decode failure is a per-child failure, not a transport error")
	require.Equal(t, 0, repo.applyCalls, "ApplyBulkOperations must never be invoked when decode fails")

	jobRepo.AssertExpectations(t)
}

// TestProductParamBulkHandler_Handle_InvalidParamID_MarksFailed proves a
// malformed param_id inside one decoded operation also surfaces as a
// per-child failure via toBulkOp, before ApplyBulkOperations is ever called.
func TestProductParamBulkHandler_Handle_InvalidParamID_MarksFailed(t *testing.T) {
	jobRepo := &workerJobRepoMock{}
	parentID := uuid.New()
	child := newProductParamBulkChild(t, parentID)

	msg := rabbitmq.JobMessage{
		JobID:        child.ID().String(),
		ProductSysID: 101,
		Operations:   opsJSON(t, productparambulk.OperationDTO{Kind: productparambulk.OpAddApplicable, ParamID: "not-a-uuid"}),
		CreatedBy:    "admin",
	}

	jobRepo.On("GetByID", mock.Anything, child.ID()).Return(child, nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.ID() == child.ID() && e.Status() == job.StatusFailed
	})).Return(nil).Once()
	jobRepo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(false, nil).Once()

	repo := &fakeCPPRepo{}
	h := NewProductParamBulkHandler(jobRepo, repo, zerolog.Nop())

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, 0, repo.applyCalls)

	jobRepo.AssertExpectations(t)
}

// TestProductParamBulkHandler_Handle_ApplyBulkOperationsError_MarksFailed
// proves a hard repository-level failure (e.g. product locked, DB error) is
// also swallowed into a FAILED child status + failure notification to the
// parent, never a returned error from Handle.
func TestProductParamBulkHandler_Handle_ApplyBulkOperationsError_MarksFailed(t *testing.T) {
	jobRepo := &workerJobRepoMock{}
	parentID := uuid.New()
	child := newProductParamBulkChild(t, parentID)

	msg := rabbitmq.JobMessage{
		JobID:        child.ID().String(),
		ProductSysID: 101,
		Operations:   opsJSON(t, productparambulk.OperationDTO{Kind: productparambulk.OpAddApplicable, ParamID: uuid.NewString()}),
		CreatedBy:    "admin",
	}

	jobRepo.On("GetByID", mock.Anything, child.ID()).Return(child, nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
		return e.ID() == child.ID() && e.Status() == job.StatusFailed
	})).Return(nil).Once()
	jobRepo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(false, nil).Once()

	repo := &fakeCPPRepo{err: errors.New("product is locked")}
	h := NewProductParamBulkHandler(jobRepo, repo, zerolog.Nop())

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, 1, repo.applyCalls)

	jobRepo.AssertExpectations(t)
}

// --- batch-completion transitions -------------------------------------------

// TestProductParamBulkHandler_ChildCompletion_TerminalStatusOnce mirrors
// TestMBBulkTransitionHandler_ChildCompletion_TerminalStatusOnce's shape for
// ProductParamBulkHandler.handleChildCompletion.
func TestProductParamBulkHandler_ChildCompletion_TerminalStatusOnce(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()

	t.Run("batch not yet complete: parent job row untouched", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}
		jobRepo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(false, nil).Once()

		h := NewProductParamBulkHandler(jobRepo, &fakeCPPRepo{}, zerolog.Nop())
		child := newProductParamBulkChild(t, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), ProductSysID: 1, CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), child, msg, true)

		jobRepo.AssertExpectations(t)
		jobRepo.AssertNotCalled(t, "GetByID", mock.Anything, parentID)
	})

	t.Run("batch complete, all succeeded: parent marked COMPLETE exactly once", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}
		parent := newProductParamBulkParent(t, parentID, 3, 2, 0)

		jobRepo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(true, nil).Once()
		jobRepo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
		jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
			return e.ID() == parentID && e.Status() == job.StatusSuccess
		})).Return(nil).Once()

		h := NewProductParamBulkHandler(jobRepo, &fakeCPPRepo{}, zerolog.Nop())
		child := newProductParamBulkChild(t, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), ProductSysID: 1, CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), child, msg, true)

		jobRepo.AssertExpectations(t)
		jobRepo.AssertNumberOfCalls(t, "UpdateStatus", 1)
	})

	t.Run("batch complete, all children failed: parent marked FAILED exactly once", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}
		parent := newProductParamBulkParent(t, parentID, 3, 0, 3)

		jobRepo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(true, nil).Once()
		jobRepo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
		jobRepo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
			return e.ID() == parentID && e.Status() == job.StatusFailed
		})).Return(nil).Once()

		h := NewProductParamBulkHandler(jobRepo, &fakeCPPRepo{}, zerolog.Nop())
		child := newProductParamBulkChild(t, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), ProductSysID: 1, CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), child, msg, false)

		jobRepo.AssertExpectations(t)
		jobRepo.AssertNumberOfCalls(t, "UpdateStatus", 1)
	})

	t.Run("non-child job (nil parent id): no-op, no double-fire", func(t *testing.T) {
		t.Parallel()
		jobRepo := &workerJobRepoMock{}

		h := NewProductParamBulkHandler(jobRepo, &fakeCPPRepo{}, zerolog.Nop())
		standalone, err := job.NewExecution(job.TypeProductParamBulk, "", "", "admin", 5, nil)
		require.NoError(t, err)
		msg := rabbitmq.JobMessage{JobID: standalone.ID().String(), ProductSysID: 1, CreatedBy: "admin"}

		h.handleChildCompletion(context.Background(), standalone, msg, true)

		jobRepo.AssertNotCalled(t, "IncrementChildProgress", mock.Anything, mock.Anything, mock.Anything)
		jobRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
	})
}
