package rmcost_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

type presignMock struct{ mock.Mock }

func (m *presignMock) PresignedGetURL(ctx context.Context, key string, validity time.Duration, name string) (string, error) {
	args := m.Called(ctx, key, validity, name)
	return args.String(0), args.Error(1)
}

func mustExportExecution(t *testing.T, status job.Status, summary string, createdBy string) *job.Execution {
	t.Helper()
	exec, err := job.NewExecution(job.TypeRMCostExport, "xlsx", "202604", createdBy, 5, json.RawMessage(`{}`))
	require.NoError(t, err)
	switch status {
	case job.StatusProcessing:
		require.NoError(t, exec.Start())
	case job.StatusSuccess:
		require.NoError(t, exec.Start())
		require.NoError(t, exec.Complete(json.RawMessage(summary)))
	case job.StatusFailed:
		require.NoError(t, exec.Start())
		require.NoError(t, exec.Fail("err"))
	}
	return exec
}

func TestGetExportURL_Forbidden(t *testing.T) {
	t.Parallel()
	exec := mustExportExecution(t, job.StatusSuccess, `{"file_path":"exports/x.xlsx","file_name":"x.xlsx"}`, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	h := apprmcost.NewGetExportURLHandler(repo, storage, time.Minute)

	_, err := h.Handle(context.Background(), apprmcost.GetExportURLCommand{JobID: uuid.New(), UserID: "stranger"})
	require.Error(t, err)
	repo.AssertExpectations(t)
	storage.AssertNotCalled(t, "PresignedGetURL")
}

func TestGetExportURL_NotCompleted(t *testing.T) {
	t.Parallel()
	exec := mustExportExecution(t, job.StatusProcessing, "", "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	h := apprmcost.NewGetExportURLHandler(repo, storage, time.Minute)

	_, err := h.Handle(context.Background(), apprmcost.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.Error(t, err)
}

func TestGetExportURL_HappyPath(t *testing.T) {
	t.Parallel()
	summary := `{"file_path":"exports/finance/rm-cost/2026-04/u/x.xlsx","file_name":"rm-cost-202604-x.xlsx"}`
	exec := mustExportExecution(t, job.StatusSuccess, summary, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	storage.On("PresignedGetURL",
		mock.Anything,
		"exports/finance/rm-cost/2026-04/u/x.xlsx",
		time.Minute,
		"rm-cost-202604-x.xlsx",
	).Return("https://minio/presigned?abc", nil).Once()

	h := apprmcost.NewGetExportURLHandler(repo, storage, time.Minute)

	res, err := h.Handle(context.Background(), apprmcost.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.NoError(t, err)
	assert.Equal(t, "https://minio/presigned?abc", res.URL)
	assert.Equal(t, "rm-cost-202604-x.xlsx", res.FileName)
}

func TestGetExportURL_StorageNil(t *testing.T) {
	t.Parallel()
	repo := &jobRepoMock{}
	h := apprmcost.NewGetExportURLHandler(repo, nil, time.Minute)
	_, err := h.Handle(context.Background(), apprmcost.GetExportURLCommand{JobID: uuid.New(), UserID: "u"})
	require.Error(t, err)
}

func TestGetExportURL_PresignFailure(t *testing.T) {
	t.Parallel()
	exec := mustExportExecution(t, job.StatusSuccess, `{"file_path":"x","file_name":"y"}`, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	storage.On("PresignedGetURL", mock.Anything, "x", mock.Anything, "y").Return("", errors.New("minio down")).Once()

	h := apprmcost.NewGetExportURLHandler(repo, storage, time.Minute)
	_, err := h.Handle(context.Background(), apprmcost.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.Error(t, err)
}
