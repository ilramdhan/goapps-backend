package costfillassignment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// ---------------------------------------------------------------------------
// mock repositories
// ---------------------------------------------------------------------------

type mockConfigRepo struct {
	global   *domain.Config
	getErr   error
	upserted bool
}

func (m *mockConfigRepo) UpsertGlobal(_ context.Context, _ *domain.Config, _ string) error {
	m.upserted = true
	return nil
}

func (m *mockConfigRepo) DeleteGlobal(_ context.Context, _ int32) error { return nil }

func (m *mockConfigRepo) ListGlobal(_ context.Context) ([]*domain.Config, error) {
	if m.global != nil {
		return []*domain.Config{m.global}, nil
	}
	return nil, nil
}

func (m *mockConfigRepo) GetGlobal(_ context.Context, _ int32) (*domain.Config, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.global, nil
}

func (m *mockConfigRepo) UpsertProduct(_ context.Context, _ *domain.Config, _ string) error {
	return nil
}

func (m *mockConfigRepo) GetProduct(_ context.Context, _ int64, _ int32) (*domain.Config, error) {
	return nil, nil
}

func (m *mockConfigRepo) UpsertRequest(_ context.Context, _ *domain.Config, _ string) error {
	return nil
}

func (m *mockConfigRepo) GetRequest(_ context.Context, _ int64, _ int32) (*domain.Config, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------

type mockTaskRepo struct {
	tasks   []*domain.Task
	claimOK bool
	saveErr error
}

func (m *mockTaskRepo) BulkInsert(_ context.Context, _ []*domain.Task) error { return nil }

func (m *mockTaskRepo) GetByID(_ context.Context, _ int64) (*domain.Task, error) {
	if len(m.tasks) > 0 {
		return m.tasks[0], nil
	}
	return nil, domain.ErrTaskNotFound
}

func (m *mockTaskRepo) GetByRequestLevel(_ context.Context, _ int64, _ int32) (*domain.Task, error) {
	return nil, domain.ErrTaskNotFound
}

func (m *mockTaskRepo) ListByRequest(_ context.Context, _ int64) ([]*domain.Task, error) {
	return m.tasks, nil
}

func (m *mockTaskRepo) ListForUser(_ context.Context, _ string, _ []string) ([]*domain.Task, error) {
	return m.tasks, nil
}

func (m *mockTaskRepo) Claim(_ context.Context, _ int64, _ string) (bool, error) {
	return m.claimOK, nil
}

func (m *mockTaskRepo) Save(_ context.Context, _ *domain.Task) error { return m.saveErr }

func (m *mockTaskRepo) IncrementFilled(_ context.Context, _ int64, _ int32, _ int32) (*domain.Task, error) {
	if len(m.tasks) > 0 {
		return m.tasks[0], nil
	}
	return nil, domain.ErrTaskNotFound
}

func (m *mockTaskRepo) CountNonApproved(_ context.Context, _ int64) (int, error) { return 0, nil }

func (m *mockTaskRepo) MarkNotified(_ context.Context, _ int64) error { return nil }

func (m *mockTaskRepo) ListOverdue(_ context.Context, _ int) ([]*domain.Task, error) {
	return nil, nil
}

func (m *mockTaskRepo) AddApproval(_ context.Context, _ *domain.Approval) error { return nil }

func (m *mockTaskRepo) ListApprovals(_ context.Context, _ int64) ([]*domain.Approval, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------

type mockCompletionGate struct{ called bool }

func (m *mockCompletionGate) CheckAndAdvance(_ context.Context, _ int64) error {
	m.called = true
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

const (
	testFillerType  = "USER"
	testFillerValue = "u1"
	testActor       = "admin"
)

func newFillingTask() *domain.Task {
	rc := domain.ResolvedConfig{
		RouteLevel: 1, FillerType: testFillerType, FillerValue: testFillerValue,
		SLAFillHours: 48, SLAApproveHours: 24,
	}
	t := domain.NewTask(1, 10, rc, 5)
	_ = t.Claim("u-filler") // ACTIVE → FILLING
	return t
}

func newFillingTaskWithApprover() *domain.Task {
	rc := domain.ResolvedConfig{
		RouteLevel: 1, FillerType: testFillerType, FillerValue: testFillerValue,
		ApproverType: "USER", ApproverValue: "u-boss",
		SLAFillHours: 48, SLAApproveHours: 24,
	}
	t := domain.NewTask(1, 10, rc, 5)
	_ = t.Claim("u-filler") // ACTIVE → FILLING
	return t
}

func newApprovalPendingTask() *domain.Task {
	t := newFillingTaskWithApprover()
	_ = t.Submit() // FILLING → APPROVAL_PENDING
	return t
}

// ---------------------------------------------------------------------------
// UpsertGlobalConfigHandler
// ---------------------------------------------------------------------------

func TestUpsertGlobalConfigHandler_Valid(t *testing.T) {
	repo := &mockConfigRepo{}
	h := app.NewUpsertGlobalConfigHandler(repo)
	err := h.Handle(context.Background(), app.UpsertGlobalConfigCommand{
		RouteLevel:  1,
		FillerType:  testFillerType,
		FillerValue: testFillerValue,
		Actor:       testActor,
	})
	require.NoError(t, err)
	assert.True(t, repo.upserted)
}

func TestUpsertGlobalConfigHandler_ZeroLevel(t *testing.T) {
	h := app.NewUpsertGlobalConfigHandler(&mockConfigRepo{})
	err := h.Handle(context.Background(), app.UpsertGlobalConfigCommand{
		RouteLevel:  0,
		FillerType:  testFillerType,
		FillerValue: testFillerValue,
		Actor:       testActor,
	})
	require.Error(t, err)
}

func TestUpsertGlobalConfigHandler_MissingFiller(t *testing.T) {
	h := app.NewUpsertGlobalConfigHandler(&mockConfigRepo{})
	err := h.Handle(context.Background(), app.UpsertGlobalConfigCommand{
		RouteLevel:  1,
		FillerType:  "",
		FillerValue: testFillerValue,
		Actor:       testActor,
	})
	require.Error(t, err)
}

func TestUpsertGlobalConfigHandler_MissingActor(t *testing.T) {
	h := app.NewUpsertGlobalConfigHandler(&mockConfigRepo{})
	err := h.Handle(context.Background(), app.UpsertGlobalConfigCommand{
		RouteLevel:  1,
		FillerType:  testFillerType,
		FillerValue: testFillerValue,
		Actor:       "",
	})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// ListGlobalConfigHandler
// ---------------------------------------------------------------------------

func TestListGlobalConfigHandler_ReturnsConfigs(t *testing.T) {
	ft := testFillerType
	fv := testFillerValue
	repo := &mockConfigRepo{global: &domain.Config{
		Tier: domain.TierGlobal, RouteLevel: 1,
		FillerType: &ft, FillerValue: &fv,
	}}
	h := app.NewListGlobalConfigHandler(repo)
	result, err := h.Handle(context.Background(), app.ListGlobalConfigQuery{})
	require.NoError(t, err)
	assert.Len(t, result.Configs, 1)
}

func TestListGlobalConfigHandler_Empty(t *testing.T) {
	h := app.NewListGlobalConfigHandler(&mockConfigRepo{})
	result, err := h.Handle(context.Background(), app.ListGlobalConfigQuery{})
	require.NoError(t, err)
	assert.Empty(t, result.Configs)
}

// ---------------------------------------------------------------------------
// ClaimTaskHandler
// ---------------------------------------------------------------------------

func TestClaimTaskHandler_Success(t *testing.T) {
	repo := &mockTaskRepo{claimOK: true}
	h := app.NewClaimTaskHandler(repo)
	err := h.Handle(context.Background(), app.ClaimTaskCommand{TaskID: 1, UserID: "u1"})
	require.NoError(t, err)
}

func TestClaimTaskHandler_AlreadyClaimed(t *testing.T) {
	repo := &mockTaskRepo{claimOK: false}
	h := app.NewClaimTaskHandler(repo)
	err := h.Handle(context.Background(), app.ClaimTaskCommand{TaskID: 1, UserID: "u1"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAlreadyClaimed))
}

func TestClaimTaskHandler_InvalidTaskID(t *testing.T) {
	h := app.NewClaimTaskHandler(&mockTaskRepo{})
	err := h.Handle(context.Background(), app.ClaimTaskCommand{TaskID: 0, UserID: "u1"})
	require.Error(t, err)
}

func TestClaimTaskHandler_MissingUserID(t *testing.T) {
	h := app.NewClaimTaskHandler(&mockTaskRepo{})
	err := h.Handle(context.Background(), app.ClaimTaskCommand{TaskID: 1, UserID: ""})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// ListTasksHandler
// ---------------------------------------------------------------------------

func TestListTasksHandler_ReturnsTasks(t *testing.T) {
	repo := &mockTaskRepo{tasks: []*domain.Task{newFillingTask()}}
	h := app.NewListTasksHandler(repo)
	result, err := h.Handle(context.Background(), app.ListTasksQuery{RequestID: 1})
	require.NoError(t, err)
	assert.Len(t, result.Tasks, 1)
}

func TestListTasksHandler_Empty(t *testing.T) {
	h := app.NewListTasksHandler(&mockTaskRepo{})
	result, err := h.Handle(context.Background(), app.ListTasksQuery{RequestID: 1})
	require.NoError(t, err)
	assert.Empty(t, result.Tasks)
}

func TestListTasksHandler_ZeroRequestID(t *testing.T) {
	h := app.NewListTasksHandler(&mockTaskRepo{})
	_, err := h.Handle(context.Background(), app.ListTasksQuery{RequestID: 0})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// SubmitFillHandler
// ---------------------------------------------------------------------------

func TestSubmitFillHandler_NoApprover_CallsGate(t *testing.T) {
	// No approver → Submit() goes straight to APPROVED → gate must fire.
	task := newFillingTask() // FILLING, no approver
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	gate := &mockCompletionGate{}
	h := app.NewSubmitFillHandler(repo, gate)
	err := h.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u-filler"})
	require.NoError(t, err)
	assert.True(t, gate.called, "completion gate should be called after auto-approve")
}

func TestSubmitFillHandler_WithApprover_GateNotCalled(t *testing.T) {
	// With approver → Submit() goes to APPROVAL_PENDING → gate must NOT fire.
	task := newFillingTaskWithApprover() // FILLING, has approver
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	gate := &mockCompletionGate{}
	h := app.NewSubmitFillHandler(repo, gate)
	err := h.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u-filler"})
	require.NoError(t, err)
	assert.False(t, gate.called, "gate must not fire when task is APPROVAL_PENDING")
}

func TestSubmitFillHandler_TaskNotFound(t *testing.T) {
	repo := &mockTaskRepo{} // empty → GetByID returns ErrTaskNotFound
	gate := &mockCompletionGate{}
	h := app.NewSubmitFillHandler(repo, gate)
	err := h.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u"})
	require.Error(t, err)
}

func TestSubmitFillHandler_InvalidTransition(t *testing.T) {
	// Task is ACTIVE (not FILLING) → Submit() returns ErrInvalidTransition.
	rc := domain.ResolvedConfig{RouteLevel: 1, FillerType: testFillerType, FillerValue: testFillerValue}
	task := domain.NewTask(1, 10, rc, 5) // status=ACTIVE, not FILLING
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	gate := &mockCompletionGate{}
	h := app.NewSubmitFillHandler(repo, gate)
	err := h.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u"})
	require.Error(t, err)
}

func TestSubmitFillHandler_ZeroTaskID(t *testing.T) {
	h := app.NewSubmitFillHandler(&mockTaskRepo{}, &mockCompletionGate{})
	err := h.Handle(context.Background(), app.SubmitFillCommand{TaskID: 0, RequestID: 1, UserID: "u"})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// ApproveTaskHandler
// ---------------------------------------------------------------------------

func TestApproveTaskHandler_Success_CallsGate(t *testing.T) {
	task := newApprovalPendingTask()
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	gate := &mockCompletionGate{}
	h := app.NewApproveTaskHandler(repo, gate)
	err := h.Handle(context.Background(), app.ApproveTaskCommand{
		TaskID: 1, RequestID: 1, ApproverID: "u-boss", Note: "LGTM",
	})
	require.NoError(t, err)
	assert.True(t, gate.called, "gate must be called after approve")
}

func TestApproveTaskHandler_WrongStatus(t *testing.T) {
	// Task is FILLING → Approve() returns ErrInvalidTransition.
	task := newFillingTaskWithApprover()
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	h := app.NewApproveTaskHandler(repo, &mockCompletionGate{})
	err := h.Handle(context.Background(), app.ApproveTaskCommand{
		TaskID: 1, RequestID: 1, ApproverID: "u-boss",
	})
	require.Error(t, err)
}

func TestApproveTaskHandler_MissingApproverID(t *testing.T) {
	h := app.NewApproveTaskHandler(&mockTaskRepo{}, &mockCompletionGate{})
	err := h.Handle(context.Background(), app.ApproveTaskCommand{TaskID: 1, RequestID: 1, ApproverID: ""})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// RejectTaskHandler
// ---------------------------------------------------------------------------

func TestRejectTaskHandler_Success(t *testing.T) {
	task := newApprovalPendingTask()
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	h := app.NewRejectTaskHandler(repo)
	err := h.Handle(context.Background(), app.RejectTaskCommand{
		TaskID: 1, ApproverID: "u-boss", Reason: "wrong qty",
	})
	require.NoError(t, err)
}

func TestRejectTaskHandler_WrongStatus(t *testing.T) {
	// Task is FILLING → Reject() returns ErrInvalidTransition.
	task := newFillingTaskWithApprover()
	repo := &mockTaskRepo{tasks: []*domain.Task{task}}
	h := app.NewRejectTaskHandler(repo)
	err := h.Handle(context.Background(), app.RejectTaskCommand{
		TaskID: 1, ApproverID: "u-boss", Reason: "wrong qty",
	})
	require.Error(t, err)
}

func TestRejectTaskHandler_MissingApproverID(t *testing.T) {
	h := app.NewRejectTaskHandler(&mockTaskRepo{})
	err := h.Handle(context.Background(), app.RejectTaskCommand{TaskID: 1, ApproverID: ""})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Full lifecycle: claim → submit → approve (through all handlers end-to-end)
// ---------------------------------------------------------------------------

func TestFillLifecycle_ClaimSubmitApprove(t *testing.T) {
	// The ClaimTaskHandler delegates to repo.Claim() (an atomic DB UPDATE) and does
	// not mutate the in-memory task. We verify the handler reports success and then
	// advance the domain object manually (simulating what the DB row would reflect
	// on the next GetByID call) so the subsequent Submit and Approve steps work.
	rc := domain.ResolvedConfig{
		RouteLevel: 1, FillerType: testFillerType, FillerValue: "u-filler",
		ApproverType: "USER", ApproverValue: "u-boss",
		SLAFillHours: 48, SLAApproveHours: 24,
	}
	task := domain.NewTask(1, 10, rc, 3)
	repo := &mockTaskRepo{claimOK: true, tasks: []*domain.Task{task}}
	gate := &mockCompletionGate{}

	claimH := app.NewClaimTaskHandler(repo)
	submitH := app.NewSubmitFillHandler(repo, gate)
	approveH := app.NewApproveTaskHandler(repo, gate)

	// Step 1: claim handler succeeds (atomic DB claim) → advance domain object.
	require.NoError(t, claimH.Handle(context.Background(), app.ClaimTaskCommand{TaskID: 1, UserID: "u-filler"}))
	require.NoError(t, task.Claim("u-filler")) // reflect DB state in the mock object
	assert.Equal(t, domain.StatusFilling, task.Status())

	// Step 2: submit → APPROVAL_PENDING (has approver), gate not called.
	require.NoError(t, submitH.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u-filler"}))
	assert.Equal(t, domain.StatusApprovalPending, task.Status())
	assert.False(t, gate.called)

	// Step 3: approve → APPROVED, gate called.
	require.NoError(t, approveH.Handle(context.Background(), app.ApproveTaskCommand{
		TaskID: 1, RequestID: 1, ApproverID: "u-boss", Note: "all good",
	}))
	assert.Equal(t, domain.StatusApproved, task.Status())
	assert.True(t, gate.called)
}

func TestFillLifecycle_ClaimSubmitRejectResubmitApprove(t *testing.T) {
	rc := domain.ResolvedConfig{
		RouteLevel: 1, FillerType: testFillerType, FillerValue: "u-filler",
		ApproverType: "USER", ApproverValue: "u-boss",
		SLAFillHours: 48, SLAApproveHours: 24,
	}
	task := domain.NewTask(1, 10, rc, 3)
	repo := &mockTaskRepo{claimOK: true, tasks: []*domain.Task{task}}
	gate := &mockCompletionGate{}

	submitH := app.NewSubmitFillHandler(repo, gate)
	rejectH := app.NewRejectTaskHandler(repo)
	approveH := app.NewApproveTaskHandler(repo, gate)

	// Manually claim to put the task in FILLING state (simulates a prior DB claim).
	require.NoError(t, task.Claim("u-filler"))
	assert.Equal(t, domain.StatusFilling, task.Status())

	// Submit → APPROVAL_PENDING.
	require.NoError(t, submitH.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u-filler"}))
	assert.Equal(t, domain.StatusApprovalPending, task.Status())

	// Reject → REJECTED.
	require.NoError(t, rejectH.Handle(context.Background(), app.RejectTaskCommand{TaskID: 1, ApproverID: "u-boss", Reason: "fix it"}))
	assert.Equal(t, domain.StatusRejected, task.Status())

	// Resubmit (domain only — the gRPC handler calls Resubmit + repo.Save directly).
	require.NoError(t, task.Resubmit())
	assert.Equal(t, domain.StatusFilling, task.Status())

	// Submit again → APPROVAL_PENDING.
	require.NoError(t, submitH.Handle(context.Background(), app.SubmitFillCommand{TaskID: 1, RequestID: 1, UserID: "u-filler"}))
	assert.Equal(t, domain.StatusApprovalPending, task.Status())

	// Approve → APPROVED, gate fires.
	require.NoError(t, approveH.Handle(context.Background(), app.ApproveTaskCommand{
		TaskID: 1, RequestID: 1, ApproverID: "u-boss", Note: "now OK",
	}))
	assert.Equal(t, domain.StatusApproved, task.Status())
	assert.True(t, gate.called)
}
