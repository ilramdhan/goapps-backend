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
// configurable task repo for CompletionGate tests
// ---------------------------------------------------------------------------

type gateTaskRepo struct {
	tasks                 []*domain.Task
	countNonApprovedBelow int
	countErr              error
	getByLevelTask        *domain.Task
	getByLevelErr         error
	activateCalled        bool
	bulkInserted          []*domain.Task
}

func (r *gateTaskRepo) BulkInsert(_ context.Context, tasks []*domain.Task) error {
	r.bulkInserted = tasks
	return nil
}
func (r *gateTaskRepo) GetByID(_ context.Context, _ int64) (*domain.Task, error) {
	return nil, domain.ErrTaskNotFound
}
func (r *gateTaskRepo) GetByRequestLevel(_ context.Context, _ int64, _ int32) (*domain.Task, error) {
	return r.getByLevelTask, r.getByLevelErr
}
func (r *gateTaskRepo) ListByRequest(_ context.Context, _ int64) ([]*domain.Task, error) {
	return r.tasks, nil
}
func (r *gateTaskRepo) ListForUser(_ context.Context, _ string, _ []string) ([]*domain.Task, error) {
	return nil, nil
}
func (r *gateTaskRepo) Claim(_ context.Context, _ int64, _ string) (bool, error) { return true, nil }
func (r *gateTaskRepo) Save(_ context.Context, _ *domain.Task) error             { return nil }
func (r *gateTaskRepo) IncrementFilled(_ context.Context, _ int64, _ int32, _ int32) (*domain.Task, error) {
	return nil, nil
}
func (r *gateTaskRepo) CountNonApproved(_ context.Context, _ int64) (int, error) { return 0, nil }
func (r *gateTaskRepo) CountNonApprovedBelow(_ context.Context, _ int64, _ int32) (int, error) {
	return r.countNonApprovedBelow, r.countErr
}
func (r *gateTaskRepo) ActivateTask(_ context.Context, _ int64) error {
	r.activateCalled = true
	return nil
}
func (r *gateTaskRepo) MarkNotified(_ context.Context, _ int64) error { return nil }
func (r *gateTaskRepo) ListOverdue(_ context.Context, _ int) ([]*domain.Task, error) {
	return nil, nil
}
func (r *gateTaskRepo) ListPendingFill(_ context.Context, _ int) ([]*domain.Task, error) {
	return nil, nil
}
func (r *gateTaskRepo) ListPendingApproval(_ context.Context, _ int) ([]*domain.Task, error) {
	return nil, nil
}
func (r *gateTaskRepo) AddApproval(_ context.Context, _ *domain.Approval) error { return nil }
func (r *gateTaskRepo) ListApprovals(_ context.Context, _ int64) ([]*domain.Approval, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// configurable config repo
// ---------------------------------------------------------------------------

type gateConfigRepo struct {
	globalByLevel map[int32]*domain.Config
}

func (r *gateConfigRepo) UpsertGlobal(_ context.Context, _ *domain.Config, _ string) error {
	return nil
}
func (r *gateConfigRepo) DeleteGlobal(_ context.Context, _ int32) error { return nil }
func (r *gateConfigRepo) ListGlobal(_ context.Context) ([]*domain.Config, error) {
	return nil, nil
}
func (r *gateConfigRepo) GetGlobal(_ context.Context, level int32) (*domain.Config, error) {
	if r.globalByLevel == nil {
		return nil, domain.ErrConfigNotFound
	}
	c, ok := r.globalByLevel[level]
	if !ok {
		return nil, domain.ErrConfigNotFound
	}
	return c, nil
}
func (r *gateConfigRepo) UpsertProduct(_ context.Context, _ *domain.Config, _ string) error {
	return nil
}
func (r *gateConfigRepo) GetProduct(_ context.Context, _ int64, _ int32) (*domain.Config, error) {
	return nil, nil
}
func (r *gateConfigRepo) UpsertRequest(_ context.Context, _ *domain.Config, _ string) error {
	return nil
}
func (r *gateConfigRepo) GetRequest(_ context.Context, _ int64, _ int32) (*domain.Config, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// CPRCompleter + CompletionNotifier stubs
// ---------------------------------------------------------------------------

type stubCompleter struct {
	called      bool
	requesterID string
	requestNo   string
	err         error
}

func (s *stubCompleter) MarkParameterComplete(_ context.Context, _ int64, _ string) (string, string, error) {
	s.called = true
	return s.requesterID, s.requestNo, s.err
}

type stubNotifier struct {
	fillerCalled   bool
	completeCalled bool
}

func (s *stubNotifier) NotifyFiller(_ context.Context, _ int64, _, _ string) error {
	s.fillerCalled = true
	return nil
}
func (s *stubNotifier) NotifyComplete(_ context.Context, _ int64, _, _ string) error {
	s.completeCalled = true
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func buildExistingTask(routeHeadID, requestID int64, level int32) *domain.Task {
	rc := domain.ResolvedConfig{
		RouteLevel:      level,
		FillerType:      domain.ActorDept,
		FillerValue:     "PROD",
		SLAFillHours:    48,
		SLAApproveHours: 0,
	}
	return domain.NewTask(requestID, routeHeadID, rc, 0)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCompletionGate_RegularLevel_NotAllApproved(t *testing.T) {
	taskRepo := &gateTaskRepo{
		countNonApprovedBelow: 2, // still 2 non-approved tasks
		tasks:                 []*domain.Task{buildExistingTask(10, 1, 1)},
	}
	gate := app.NewCompletionGateHandler(taskRepo, &gateConfigRepo{}, nil, nil)

	err := gate.CheckAndAdvance(context.Background(), 1, 1)

	require.NoError(t, err)
	assert.Nil(t, taskRepo.bulkInserted, "must not create any tasks")
	assert.False(t, taskRepo.activateCalled, "must not activate any task")
}

func TestCompletionGate_RegularLevel_AllApproved_CallsMarkComplete(t *testing.T) {
	completer := &stubCompleter{requesterID: "user-1", requestNo: "REQ-001"}
	notifier := &stubNotifier{}
	taskRepo := &gateTaskRepo{
		countNonApprovedBelow: 0,
		tasks:                 []*domain.Task{buildExistingTask(10, 1, 1)},
	}
	gate := app.NewCompletionGateHandler(taskRepo, &gateConfigRepo{}, completer, notifier)

	err := gate.CheckAndAdvance(context.Background(), 1, 1)

	require.NoError(t, err)
	assert.True(t, completer.called, "must call MarkParameterComplete when all levels approved")
	assert.True(t, notifier.completeCalled, "must notify requester on completion")
}

func TestCompletionGate_RegularLevel_AllApproved_NilCompleter_NoError(t *testing.T) {
	taskRepo := &gateTaskRepo{
		countNonApprovedBelow: 0,
		tasks:                 []*domain.Task{buildExistingTask(10, 1, 1)},
	}
	gate := app.NewCompletionGateHandler(taskRepo, &gateConfigRepo{}, nil, nil)

	err := gate.CheckAndAdvance(context.Background(), 1, 1)

	require.NoError(t, err, "nil completer must not panic or return error")
}

func TestCompletionGate_RegularLevel_AllApproved_NoChainTasksCreated(t *testing.T) {
	// Verify that no L100/101/102 tasks are bulk-inserted even when a config
	// happens to exist for those levels (migration 000373 deletes them, but the
	// handler itself must not consult the config repo at all).
	taskRepo := &gateTaskRepo{
		countNonApprovedBelow: 0,
		tasks:                 []*domain.Task{buildExistingTask(10, 1, 1)},
	}
	completer := &stubCompleter{requesterID: "u-1", requestNo: "REQ-002"}
	gate := app.NewCompletionGateHandler(taskRepo, &gateConfigRepo{}, completer, nil)

	err := gate.CheckAndAdvance(context.Background(), 1, 1)

	require.NoError(t, err)
	assert.Nil(t, taskRepo.bulkInserted, "handler must never create L100-102 chain tasks")
	assert.False(t, taskRepo.activateCalled, "handler must never activate completion chain tasks")
	assert.True(t, completer.called, "MarkParameterComplete must be called directly")
}

func TestCompletionGate_CountError_Propagated(t *testing.T) {
	countErr := errors.New("db unavailable")
	taskRepo := &gateTaskRepo{countErr: countErr}
	gate := app.NewCompletionGateHandler(taskRepo, &gateConfigRepo{}, nil, nil)

	err := gate.CheckAndAdvance(context.Background(), 1, 1)

	require.Error(t, err)
	assert.True(t, errors.Is(err, countErr))
}

func TestCompletionGate_MarkCompleteError_Propagated(t *testing.T) {
	completeErr := errors.New("transition failed")
	taskRepo := &gateTaskRepo{countNonApprovedBelow: 0}
	completer := &stubCompleter{err: completeErr}
	gate := app.NewCompletionGateHandler(taskRepo, &gateConfigRepo{}, completer, nil)

	err := gate.CheckAndAdvance(context.Background(), 7, 5)

	require.Error(t, err)
	assert.True(t, errors.Is(err, completeErr))
}
