package costsheet_test

import (
	"context"
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

func mustParentExecution(t *testing.T, total int) *job.Execution {
	t.Helper()
	exec, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user:owner-1", 5, nil, total)
	require.NoError(t, err)
	return exec
}

func mustChildExecution(t *testing.T, parentID uuid.UUID, status job.Status, summary string) *job.Execution {
	t.Helper()
	exec, err := job.NewChildExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user:owner-1", 5, nil, parentID)
	require.NoError(t, err)
	switch status {
	case job.StatusProcessing:
		require.NoError(t, exec.Start())
	case job.StatusSuccess:
		require.NoError(t, exec.Start())
		require.NoError(t, exec.Complete([]byte(summary)))
	case job.StatusFailed:
		require.NoError(t, exec.Start())
		require.NoError(t, exec.Fail("boom"))
	case job.StatusQueued, job.StatusCancelled:
		// leave as-is / not exercised here
	}
	return exec
}

func TestListBatchChildren_ParentNotFound(t *testing.T) {
	t.Parallel()
	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(nil, job.ErrNotFound).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, &presignMock{})
	_, err := h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: uuid.New()})
	require.Error(t, err)
	require.ErrorIs(t, err, job.ErrNotFound)
	repo.AssertExpectations(t)
}

func TestListBatchChildren_NotAParentRejected(t *testing.T) {
	t.Parallel()
	// A standalone (non-parent) job must be rejected even though it exists and
	// is the right job type.
	standalone, err := job.NewExecution(job.TypeProductCostSheetExport, "xlsx", "202604", "user:owner-1", 5, nil)
	require.NoError(t, err)

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(standalone, nil).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, &presignMock{})
	_, err = h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: uuid.New()})
	require.Error(t, err)
	require.ErrorIs(t, err, job.ErrNotBatchParent)
	repo.AssertNotCalled(t, "ListChildren", mock.Anything, mock.Anything)
}

func TestListBatchChildren_WrongJobTypeRejected(t *testing.T) {
	t.Parallel()
	exec, err := job.NewParentExecution(job.TypeRMCostExport, "xlsx", "202604", "user:owner-1", 5, nil, 3)
	require.NoError(t, err)

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, mock.Anything).Return(exec, nil).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, &presignMock{})
	_, err = h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: uuid.New()})
	require.Error(t, err)
}

func TestListBatchChildren_MixedStatuses_HappyPath(t *testing.T) {
	t.Parallel()
	parentID := uuid.New()
	parent := mustParentExecution(t, 3)

	successSummary := `{"file_path":"exports/a.xlsx","file_name":"a.xlsx"}`
	childSuccess := mustChildExecution(t, parentID, job.StatusSuccess, successSummary)
	childProcessing := mustChildExecution(t, parentID, job.StatusProcessing, "")
	childFailed := mustChildExecution(t, parentID, job.StatusFailed, "")

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
	repo.On("ListChildren", mock.Anything, parentID).
		Return([]*job.Execution{childSuccess, childProcessing, childFailed}, nil).Once()

	storage := &presignMock{}
	storage.On("PresignedGetURL", mock.Anything, "exports/a.xlsx", 5*time.Minute, "a.xlsx").
		Return("https://minio/presigned?a", nil).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, storage)
	res, err := h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: parentID})
	require.NoError(t, err)
	require.Len(t, res.Children, 3)

	byID := map[uuid.UUID]appcostsheet.BatchChildResult{}
	for _, c := range res.Children {
		byID[c.JobID] = c
	}

	success := byID[childSuccess.ID()]
	assert.Equal(t, job.StatusSuccess, success.Status)
	assert.Equal(t, "https://minio/presigned?a", success.DownloadURL)
	assert.Equal(t, "a.xlsx", success.FileName)

	processing := byID[childProcessing.ID()]
	assert.Equal(t, job.StatusProcessing, processing.Status)
	assert.Empty(t, processing.DownloadURL)

	failed := byID[childFailed.ID()]
	assert.Equal(t, job.StatusFailed, failed.Status)
	assert.Empty(t, failed.DownloadURL)

	repo.AssertExpectations(t)
	storage.AssertExpectations(t)
}

func TestListBatchChildren_PresignFailureTolerated(t *testing.T) {
	t.Parallel()
	parentID := uuid.New()
	parent := mustParentExecution(t, 2)

	childOK := mustChildExecution(t, parentID, job.StatusSuccess, `{"file_path":"exports/ok.xlsx","file_name":"ok.xlsx"}`)
	childBad := mustChildExecution(t, parentID, job.StatusSuccess, `{"file_path":"exports/bad.xlsx","file_name":"bad.xlsx"}`)

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
	repo.On("ListChildren", mock.Anything, parentID).Return([]*job.Execution{childOK, childBad}, nil).Once()

	storage := &presignMock{}
	storage.On("PresignedGetURL", mock.Anything, "exports/ok.xlsx", mock.Anything, "ok.xlsx").
		Return("https://minio/presigned?ok", nil).Once()
	storage.On("PresignedGetURL", mock.Anything, "exports/bad.xlsx", mock.Anything, "bad.xlsx").
		Return("", errors.New("minio down")).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, storage)
	res, err := h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: parentID})
	require.NoError(t, err)
	require.Len(t, res.Children, 2)

	byID := map[uuid.UUID]appcostsheet.BatchChildResult{}
	for _, c := range res.Children {
		byID[c.JobID] = c
	}
	assert.Equal(t, "https://minio/presigned?ok", byID[childOK.ID()].DownloadURL)
	assert.Empty(t, byID[childBad.ID()].DownloadURL)
	assert.Empty(t, byID[childBad.ID()].FileName)

	repo.AssertExpectations(t)
	storage.AssertExpectations(t)
}

func TestListBatchChildren_InvalidResultSummaryTolerated(t *testing.T) {
	t.Parallel()
	parentID := uuid.New()
	parent := mustParentExecution(t, 1)

	child := mustChildExecution(t, parentID, job.StatusSuccess, `{"file_path":"","file_name":""}`)

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
	repo.On("ListChildren", mock.Anything, parentID).Return([]*job.Execution{child}, nil).Once()

	storage := &presignMock{}

	h := appcostsheet.NewListBatchChildrenHandler(repo, storage)
	res, err := h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: parentID})
	require.NoError(t, err)
	require.Len(t, res.Children, 1)
	assert.Empty(t, res.Children[0].DownloadURL)
	storage.AssertNotCalled(t, "PresignedGetURL", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestListBatchChildren_NilStorageSkipsPresign(t *testing.T) {
	t.Parallel()
	parentID := uuid.New()
	parent := mustParentExecution(t, 1)
	child := mustChildExecution(t, parentID, job.StatusSuccess, `{"file_path":"exports/a.xlsx","file_name":"a.xlsx"}`)

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
	repo.On("ListChildren", mock.Anything, parentID).Return([]*job.Execution{child}, nil).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, nil)
	res, err := h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: parentID})
	require.NoError(t, err)
	require.Len(t, res.Children, 1)
	assert.Empty(t, res.Children[0].DownloadURL)
}

func TestListBatchChildren_NoChildren(t *testing.T) {
	t.Parallel()
	parentID := uuid.New()
	parent := mustParentExecution(t, 3)

	repo := &jobRepoMock{}
	repo.On("GetByID", mock.Anything, parentID).Return(parent, nil).Once()
	repo.On("ListChildren", mock.Anything, parentID).Return(nil, nil).Once()

	h := appcostsheet.NewListBatchChildrenHandler(repo, &presignMock{})
	res, err := h.Handle(context.Background(), appcostsheet.ListBatchChildrenQuery{ParentJobID: parentID})
	require.NoError(t, err)
	assert.Empty(t, res.Children)
}
