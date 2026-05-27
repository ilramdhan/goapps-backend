package rmcost_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// jobRepoMock is a small testify mock for job.Repository — only the methods
// RequestExportHandler exercises are stubbed; calls to other methods will
// panic loudly if a future change adds them.
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

type exportPubMock struct{ mock.Mock }

func (m *exportPubMock) PublishRMCostExport(ctx context.Context, jobID, period, rmType, ghID, search, recipient, by string) error {
	return m.Called(ctx, jobID, period, rmType, ghID, search, recipient, by).Error(0)
}

func TestRequestExportHandler_ValidationErrors(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &exportPubMock{}
	h := apprmcost.NewRequestExportHandler(repo, pub)

	t.Run("publisher nil", func(t *testing.T) {
		hh := apprmcost.NewRequestExportHandler(repo, nil)
		_, err := hh.Handle(context.Background(), apprmcost.RequestExportCommand{Period: "202604", RequestingUserID: "u"})
		require.Error(t, err)
	})

	t.Run("missing period", func(t *testing.T) {
		_, err := h.Handle(context.Background(), apprmcost.RequestExportCommand{RequestingUserID: "u"})
		require.Error(t, err)
	})

	t.Run("missing requesting user id", func(t *testing.T) {
		_, err := h.Handle(context.Background(), apprmcost.RequestExportCommand{Period: "202604"})
		require.Error(t, err)
	})
}

func TestRequestExportHandler_HappyPath(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &exportPubMock{}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()
	pub.On("PublishRMCostExport", mock.Anything, mock.Anything, "202604", "GROUP", "g-1", "abc", "user-7", "user-7").Return(nil).Once()

	h := apprmcost.NewRequestExportHandler(repo, pub)
	res, err := h.Handle(context.Background(), apprmcost.RequestExportCommand{
		Period:           "202604",
		RMType:           "GROUP",
		GroupHeadID:      "g-1",
		Search:           "abc",
		RequestingUserID: "user-7",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, job.TypeRMCostExport, res.Execution.JobType())
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

func TestRequestExportHandler_PublishFailureMarksJobFailed(t *testing.T) {
	t.Parallel()

	repo := &jobRepoMock{}
	pub := &exportPubMock{}

	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	pub.On("PublishRMCostExport", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("rabbitmq down")).Once()
	repo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("*job.Execution")).Return(nil).Once()

	h := apprmcost.NewRequestExportHandler(repo, pub)
	_, err := h.Handle(context.Background(), apprmcost.RequestExportCommand{
		Period:           "202604",
		RequestingUserID: "user-1",
	})
	require.Error(t, err)
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}
