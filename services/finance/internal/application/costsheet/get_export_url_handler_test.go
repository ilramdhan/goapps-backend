package costsheet_test

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

	appcostsheet "github.com/mutugading/goapps-backend/services/finance/internal/application/costsheet"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

func mustCostSheetExportExecution(t *testing.T, jobType job.Type, status job.Status, summary, createdBy string) *job.Execution {
	t.Helper()
	exec, err := job.NewExecution(jobType, "xlsx", "202604", createdBy, 5, json.RawMessage(`{}`))
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

func TestGetExportURL_WrongOwnerRejected(t *testing.T) {
	t.Parallel()
	exec := mustCostSheetExportExecution(t, job.TypeProductCostSheetExport, job.StatusSuccess,
		`{"file_path":"exports/x.xlsx","file_name":"x.xlsx"}`, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	h := appcostsheet.NewGetExportURLHandler(repo, storage, time.Minute)

	_, err := h.Handle(context.Background(), appcostsheet.GetExportURLCommand{JobID: uuid.New(), UserID: "stranger"})
	require.Error(t, err)
	repo.AssertExpectations(t)
	storage.AssertNotCalled(t, "PresignedGetURL")
}

func TestGetExportURL_WrongJobTypeRejected(t *testing.T) {
	t.Parallel()
	// A rm_cost_export job (or any other job_type) must not be servable by the
	// costsheet download handler even if otherwise SUCCESS/owned.
	exec := mustCostSheetExportExecution(t, job.TypeRMCostExport, job.StatusSuccess,
		`{"file_path":"exports/x.xlsx","file_name":"x.xlsx"}`, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	h := appcostsheet.NewGetExportURLHandler(repo, storage, time.Minute)

	_, err := h.Handle(context.Background(), appcostsheet.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.Error(t, err)
	storage.AssertNotCalled(t, "PresignedGetURL")
}

func TestGetExportURL_NotCompleted(t *testing.T) {
	t.Parallel()
	exec := mustCostSheetExportExecution(t, job.TypeProductCostSheetExport, job.StatusProcessing, "", "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	h := appcostsheet.NewGetExportURLHandler(repo, storage, time.Minute)

	_, err := h.Handle(context.Background(), appcostsheet.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.Error(t, err)
}

func TestGetExportURL_HappyPath(t *testing.T) {
	t.Parallel()
	summary := `{"file_path":"exports/finance/product-cost-sheet/2026-04/u/x.xlsx","file_name":"product-cost-sheet-202604-x.xlsx"}`
	exec := mustCostSheetExportExecution(t, job.TypeProductCostSheetExport, job.StatusSuccess, summary, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	storage.On("PresignedGetURL",
		mock.Anything,
		"exports/finance/product-cost-sheet/2026-04/u/x.xlsx",
		time.Minute,
		"product-cost-sheet-202604-x.xlsx",
	).Return("https://minio/presigned?abc", nil).Once()

	h := appcostsheet.NewGetExportURLHandler(repo, storage, time.Minute)

	res, err := h.Handle(context.Background(), appcostsheet.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.NoError(t, err)
	assert.Equal(t, "https://minio/presigned?abc", res.URL)
	assert.Equal(t, "product-cost-sheet-202604-x.xlsx", res.FileName)
}

func TestGetExportURL_StorageNil(t *testing.T) {
	t.Parallel()
	repo := &jobRepoMock{}
	h := appcostsheet.NewGetExportURLHandler(repo, nil, time.Minute)
	_, err := h.Handle(context.Background(), appcostsheet.GetExportURLCommand{JobID: uuid.New(), UserID: "u"})
	require.Error(t, err)
}

func TestGetExportURL_PresignFailure(t *testing.T) {
	t.Parallel()
	exec := mustCostSheetExportExecution(t, job.TypeProductCostSheetExport, job.StatusSuccess,
		`{"file_path":"x","file_name":"y"}`, "user:owner-1")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	storage := &presignMock{}
	storage.On("PresignedGetURL", mock.Anything, "x", mock.Anything, "y").Return("", errors.New("minio down")).Once()

	h := appcostsheet.NewGetExportURLHandler(repo, storage, time.Minute)
	_, err := h.Handle(context.Background(), appcostsheet.GetExportURLCommand{JobID: uuid.New(), UserID: "owner-1"})
	require.Error(t, err)
}
