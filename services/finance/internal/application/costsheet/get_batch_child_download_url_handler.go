package costsheet

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// GetBatchChildDownloadURLQuery identifies one child job within a batch
// whose artifact download URL should be freshly presigned.
type GetBatchChildDownloadURLQuery struct {
	ParentJobID uuid.UUID
	ChildJobID  uuid.UUID
}

// GetBatchChildDownloadURLResult carries the freshly presigned URL + suggested filename.
type GetBatchChildDownloadURLResult struct {
	URL      string
	FileName string
}

// GetBatchChildDownloadURLHandler freshly presigns a download URL for one
// child job's artifact, on demand at click-time. This exists because
// ListBatchChildrenHandler presigns URLs once when the batch listing loads
// (defaultBatchChildURLValidity, 5min) and the frontend caches that listing —
// clicking download after the URL expires previously failed with MinIO
// AccessDenied. Calling this handler at click-time instead always presigns
// a fresh URL.
//
// Lifecycle: load child → verify it belongs to the given parent → verify
// status SUCCESS → parse result_summary for file_path/file_name → presign
// via storage → return.
type GetBatchChildDownloadURLHandler struct {
	jobRepo job.Repository
	storage PresignedURLProvider
}

// NewGetBatchChildDownloadURLHandler constructs the handler, reusing the
// same storage dependency as GetExportURLHandler and ListBatchChildrenHandler.
func NewGetBatchChildDownloadURLHandler(jobRepo job.Repository, storage PresignedURLProvider) *GetBatchChildDownloadURLHandler {
	return &GetBatchChildDownloadURLHandler{jobRepo: jobRepo, storage: storage}
}

// Handle returns a freshly presigned download URL for the given child job's
// export artifact.
func (h *GetBatchChildDownloadURLHandler) Handle(ctx context.Context, q GetBatchChildDownloadURLQuery) (*GetBatchChildDownloadURLResult, error) {
	if h.storage == nil {
		return nil, fmt.Errorf("storage unavailable")
	}

	child, err := h.jobRepo.GetByID(ctx, q.ChildJobID)
	if err != nil {
		return nil, fmt.Errorf("load child job: %w", err)
	}

	if err := h.checkBelongsToParent(child, q.ParentJobID); err != nil {
		return nil, err
	}

	if child.Status() != job.StatusSuccess {
		return nil, fmt.Errorf("export not ready: status=%s", child.Status())
	}

	summary, err := parseResultSummary(child.ResultSummary())
	if err != nil {
		return nil, err
	}

	url, err := h.storage.PresignedGetURL(ctx, summary.FilePath, defaultBatchChildURLValidity, summary.FileName)
	if err != nil {
		return nil, fmt.Errorf("presign url: %w", err)
	}
	return &GetBatchChildDownloadURLResult{
		URL:      url,
		FileName: summary.FileName,
	}, nil
}

// checkBelongsToParent verifies the child job actually belongs to the given
// parent batch, preventing one caller from guessing another batch's child
// job ID to obtain a download URL cross-batch.
func (h *GetBatchChildDownloadURLHandler) checkBelongsToParent(child *job.Execution, parentJobID uuid.UUID) error {
	childParent := child.ParentJobID()
	if childParent == nil || *childParent != parentJobID {
		return job.ErrNotBatchChild
	}
	return nil
}
