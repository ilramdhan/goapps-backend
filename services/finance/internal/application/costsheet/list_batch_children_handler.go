package costsheet

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// defaultBatchChildURLValidity is the presigned URL lifetime used when
// listing a batch's children. Matches the default used by
// GetExportURLHandler for a single job.
const defaultBatchChildURLValidity = 5 * time.Minute

// ListBatchChildrenQuery identifies the parent job whose children should be
// enumerated.
type ListBatchChildrenQuery struct {
	ParentJobID uuid.UUID
}

// BatchChildResult is one child job's summary, with its download URL
// populated when the artifact is ready.
type BatchChildResult struct {
	JobID       uuid.UUID
	JobCode     string
	Status      job.Status
	DownloadURL string
	FileName    string
}

// ListBatchChildrenResult carries every child job belonging to the batch.
type ListBatchChildrenResult struct {
	Children []BatchChildResult
}

// ListBatchChildrenHandler enumerates a batch-tracking parent job's child
// export jobs, resolving each SUCCESS child's download URL via the same
// presigning path GetExportURLHandler uses for a single job.
//
// Lifecycle: load parent → verify job type → verify it is actually a parent
// → list children → for each SUCCESS child, resolve its download URL,
// tolerating per-child resolution failures (log + leave URL empty) instead
// of failing the whole listing.
type ListBatchChildrenHandler struct {
	jobRepo job.Repository
	storage PresignedURLProvider
}

// NewListBatchChildrenHandler constructs the handler, reusing the same
// storage dependency as GetExportURLHandler.
func NewListBatchChildrenHandler(jobRepo job.Repository, storage PresignedURLProvider) *ListBatchChildrenHandler {
	return &ListBatchChildrenHandler{jobRepo: jobRepo, storage: storage}
}

// Handle returns every child job of the given parent, each annotated with a
// presigned download URL when its artifact is ready.
func (h *ListBatchChildrenHandler) Handle(ctx context.Context, q ListBatchChildrenQuery) (*ListBatchChildrenResult, error) {
	parent, err := h.jobRepo.GetByID(ctx, q.ParentJobID)
	if err != nil {
		return nil, fmt.Errorf("load parent job: %w", err)
	}
	if parent.JobType() != job.TypeProductCostSheetExport {
		return nil, fmt.Errorf("job %s is not a product_cost_sheet_export job", q.ParentJobID)
	}
	if !parent.IsParent() {
		return nil, job.ErrNotBatchParent
	}

	children, err := h.jobRepo.ListChildren(ctx, q.ParentJobID)
	if err != nil {
		return nil, fmt.Errorf("list child jobs: %w", err)
	}

	results := make([]BatchChildResult, 0, len(children))
	for _, child := range children {
		results = append(results, h.toChildResult(ctx, child))
	}

	return &ListBatchChildrenResult{Children: results}, nil
}

// toChildResult maps one child execution to its result shape, resolving a
// download URL for SUCCESS children. Resolution failures are logged and
// leave the URL/filename empty rather than failing the whole batch listing.
func (h *ListBatchChildrenHandler) toChildResult(ctx context.Context, child *job.Execution) BatchChildResult {
	result := BatchChildResult{
		JobID:   child.ID(),
		JobCode: child.Code().String(),
		Status:  child.Status(),
	}
	if child.Status() != job.StatusSuccess || h.storage == nil {
		return result
	}

	summary, err := parseResultSummary(child.ResultSummary())
	if err != nil {
		log.Warn().Err(err).Str("job_id", child.ID().String()).
			Msg("batch children: skip download URL, invalid result_summary")
		return result
	}

	url, err := h.storage.PresignedGetURL(ctx, summary.FilePath, defaultBatchChildURLValidity, summary.FileName)
	if err != nil {
		log.Warn().Err(err).Str("job_id", child.ID().String()).
			Msg("batch children: skip download URL, presign failed")
		return result
	}

	result.DownloadURL = url
	result.FileName = summary.FileName
	return result
}
