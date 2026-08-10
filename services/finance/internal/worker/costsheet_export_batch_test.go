package worker

// Internal test package: exercises the parent/child batch-completion logic
// in costsheet_export_handler.go (handleChildCompletion / notifyBatchComplete
// / emitBatchReadyNotification) directly, without going through Handle's full
// export pipeline (that would require a real RouteCostSheetProvider/storage).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// batchJobRepoMock is a minimal testify mock for job.Repository covering only
// what handleChildCompletion/notifyBatchComplete call.
type batchJobRepoMock struct{ mock.Mock }

func (m *batchJobRepoMock) Create(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *batchJobRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*job.Execution, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *batchJobRepoMock) GetByCode(ctx context.Context, code string) (*job.Execution, error) {
	args := m.Called(ctx, code)
	if v := args.Get(0); v != nil {
		return v.(*job.Execution), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *batchJobRepoMock) List(ctx context.Context, f job.ListFilter) ([]*job.Execution, int64, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*job.Execution), args.Get(1).(int64), args.Error(2)
}

func (m *batchJobRepoMock) UpdateStatus(ctx context.Context, e *job.Execution) error {
	return m.Called(ctx, e).Error(0)
}

func (m *batchJobRepoMock) UpdateProgress(ctx context.Context, id uuid.UUID, p int) error {
	return m.Called(ctx, id, p).Error(0)
}

func (m *batchJobRepoMock) AddLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *batchJobRepoMock) UpdateLog(ctx context.Context, l *job.ExecutionLog) error {
	return m.Called(ctx, l).Error(0)
}

func (m *batchJobRepoMock) HasActiveJob(ctx context.Context, t job.Type, p string) (bool, error) {
	args := m.Called(ctx, t, p)
	return args.Bool(0), args.Error(1)
}

func (m *batchJobRepoMock) GetNextSequence(ctx context.Context, t job.Type, p string) (int, error) {
	args := m.Called(ctx, t, p)
	return args.Int(0), args.Error(1)
}

func (m *batchJobRepoMock) CreateChildren(ctx context.Context, execs []*job.Execution) error {
	return m.Called(ctx, execs).Error(0)
}

func (m *batchJobRepoMock) IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (bool, error) {
	args := m.Called(ctx, parentJobID, success)
	return args.Bool(0), args.Error(1)
}

func (m *batchJobRepoMock) ListChildren(ctx context.Context, parentJobID uuid.UUID) ([]*job.Execution, error) {
	args := m.Called(ctx, parentJobID)
	var out []*job.Execution
	if v := args.Get(0); v != nil {
		out = v.([]*job.Execution)
	}
	return out, args.Error(1)
}

// batchNotifMock is a testify mock for iamclient.NotificationClient.
type batchNotifMock struct{ mock.Mock }

func (m *batchNotifMock) Create(ctx context.Context, p iamclient.CreateNotificationParams) error {
	return m.Called(ctx, p).Error(0)
}

func (m *batchNotifMock) RequestNotification(ctx context.Context, p iamclient.RequestNotificationParams) error {
	return m.Called(ctx, p).Error(0)
}

func (m *batchNotifMock) Close() error {
	return m.Called().Error(0)
}

func newChildExec(t *testing.T, parentID uuid.UUID) *job.Execution {
	t.Helper()
	exec, err := job.NewChildExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user-1", 5, nil, parentID)
	require.NoError(t, err)
	require.NoError(t, exec.Start())
	return exec
}

func newParentExec(t *testing.T, id uuid.UUID, total, completed, failed int) *job.Execution {
	t.Helper()
	code, err := job.NewCode("EXP-202604-1")
	require.NoError(t, err)
	exec := job.Reconstitute(
		id, code, job.TypeProductCostSheetExport, "xlsx",
		"202604", job.StatusProcessing, 5,
		nil, nil, "",
		0, 0, 3,
		time.Now(), nil, nil,
		"user-1", "", nil,
		nil,
		nil, total, completed, failed,
	)
	return exec
}

// TestCostSheetExportHandler_ChildCompletion_NotifiesExactlyOnceWhenBatchDone
// verifies the increment-then-check-completion flow: a child finishing when
// the batch is NOT yet complete must not fire any notification; the child
// that completes the batch must fire exactly one.
func TestCostSheetExportHandler_ChildCompletion_NotifiesExactlyOnceWhenBatchDone(t *testing.T) {
	t.Parallel()

	parentID := uuid.New()

	t.Run("batch not yet complete: no notification", func(t *testing.T) {
		t.Parallel()
		repo := &batchJobRepoMock{}
		notif := &batchNotifMock{}
		repo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(false, nil).Once()

		h := NewCostSheetExportHandler(repo, nil, nil, notif, zerolog.Nop(), "")
		child := newChildExec(t, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), Period: "202604", RequestingUserID: "user-1"}

		h.handleChildCompletion(context.Background(), child, msg, true)

		repo.AssertExpectations(t)
		notif.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("batch complete: exactly one notification, parent marked COMPLETED", func(t *testing.T) {
		t.Parallel()
		repo := &batchJobRepoMock{}
		notif := &batchNotifMock{}
		parent := newParentExec(t, parentID, 3, 2, 0)

		repo.On("IncrementChildProgress", mock.Anything, parentID, true).Return(true, nil).Once()
		repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
		repo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
			return e.ID() == parentID && e.Status() == job.StatusSuccess
		})).Return(nil).Once()
		notif.On("Create", mock.Anything, mock.MatchedBy(func(p iamclient.CreateNotificationParams) bool {
			return p.SourceID == parentID.String() &&
				p.Type == iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY &&
				p.Severity == iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS
		})).Return(nil).Once()

		h := NewCostSheetExportHandler(repo, nil, nil, notif, zerolog.Nop(), "")
		child := newChildExec(t, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), Period: "202604", RequestingUserID: "user-1"}

		h.handleChildCompletion(context.Background(), child, msg, true)

		repo.AssertExpectations(t)
		notif.AssertExpectations(t)
		notif.AssertNumberOfCalls(t, "Create", 1)
	})

	t.Run("batch complete with all children failed: failure notification, parent marked FAILED", func(t *testing.T) {
		t.Parallel()
		repo := &batchJobRepoMock{}
		notif := &batchNotifMock{}
		parent := newParentExec(t, parentID, 3, 0, 3)

		repo.On("IncrementChildProgress", mock.Anything, parentID, false).Return(true, nil).Once()
		repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
		repo.On("UpdateStatus", mock.Anything, mock.MatchedBy(func(e *job.Execution) bool {
			return e.ID() == parentID && e.Status() == job.StatusFailed
		})).Return(nil).Once()
		notif.On("Create", mock.Anything, mock.MatchedBy(func(p iamclient.CreateNotificationParams) bool {
			return p.SourceID == parentID.String() &&
				p.Severity == iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_ERROR
		})).Return(nil).Once()

		h := NewCostSheetExportHandler(repo, nil, nil, notif, zerolog.Nop(), "")
		child := newChildExec(t, parentID)
		msg := rabbitmq.JobMessage{JobID: child.ID().String(), Period: "202604", RequestingUserID: "user-1"}

		h.handleChildCompletion(context.Background(), child, msg, false)

		repo.AssertExpectations(t)
		notif.AssertExpectations(t)
		notif.AssertNumberOfCalls(t, "Create", 1)
	})

	t.Run("non-child job (nil parent id): no-op", func(t *testing.T) {
		t.Parallel()
		repo := &batchJobRepoMock{}
		notif := &batchNotifMock{}

		h := NewCostSheetExportHandler(repo, nil, nil, notif, zerolog.Nop(), "")
		standalone, err := job.NewExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user-1", 5, nil)
		require.NoError(t, err)
		msg := rabbitmq.JobMessage{JobID: standalone.ID().String(), Period: "202604", RequestingUserID: "user-1"}

		h.handleChildCompletion(context.Background(), standalone, msg, true)

		repo.AssertNotCalled(t, "IncrementChildProgress", mock.Anything, mock.Anything, mock.Anything)
		notif.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}
