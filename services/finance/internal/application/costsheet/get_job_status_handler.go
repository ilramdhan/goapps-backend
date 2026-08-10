package costsheet

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// GetJobStatusQuery identifies the export job (standalone or batch parent)
// whose live status/progress should be polled.
type GetJobStatusQuery struct {
	JobID uuid.UUID
}

// GetJobStatusResult carries the job's current status and, for a batch
// parent, its live child-completion counters. Non-batch jobs report
// IsBatch=false with all counters at zero.
type GetJobStatusResult struct {
	Execution *job.Execution
}

// GetJobStatusHandler polls a product cost sheet export job's live
// status/progress — reusable for both a standalone job and a batch-tracking
// parent, since job_execution's counters (jex_completed_children /
// jex_total_children / jex_failed_children) are updated atomically by the
// worker as children finish regardless of which kind of job this is.
//
// Lifecycle: load job → verify job type → return. Unlike
// ListBatchChildrenHandler, no IsParent() check — this endpoint is
// deliberately usable for both standalone and parent jobs in one call.
type GetJobStatusHandler struct {
	jobRepo job.Repository
}

// NewGetJobStatusHandler constructs the handler.
func NewGetJobStatusHandler(jobRepo job.Repository) *GetJobStatusHandler {
	return &GetJobStatusHandler{jobRepo: jobRepo}
}

// Handle returns the job's current status/progress snapshot.
func (h *GetJobStatusHandler) Handle(ctx context.Context, q GetJobStatusQuery) (*GetJobStatusResult, error) {
	exec, err := h.jobRepo.GetByID(ctx, q.JobID)
	if err != nil {
		return nil, fmt.Errorf("load job: %w", err)
	}
	if exec.JobType() != job.TypeProductCostSheetExport {
		return nil, fmt.Errorf("job %s is not a product_cost_sheet_export job", q.JobID)
	}
	return &GetJobStatusResult{Execution: exec}, nil
}
