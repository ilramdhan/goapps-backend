package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsyncjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// ProductLookupSyncHandler processes one master-lookup-sync child job: it
// decodes the shared scope carried on msg.LookupSyncScope and calls
// lookupsync.ProductApplier.ApplyProduct for msg.ProductSysID (one transaction
// per product, FOR UPDATE lock + re-plan inside). A skipped product (locked /
// vanished) completes the child with the skip reason in its result summary —
// not a failure. Any error fails only that child. Progress is reported to the
// parent via job.Repository.IncrementChildProgress, mirroring
// ProductParamBulkHandler's lifecycle.
//
// No cost audit log entry is written: product_param_bulk does not emit one
// either; the per-child result summary (values written / cleared / skip) plus
// cpp_filled_by='lookup_sync' and cpp_updated_by=actor are the audit trail.
//
// Registered SEQUENTIALLY in cmd/worker/main.go, same conservative default as
// product_param_bulk.
type ProductLookupSyncHandler struct {
	jobRepo job.Repository
	applier lookupsync.ProductApplier
	logger  zerolog.Logger
}

// NewProductLookupSyncHandler constructs the handler.
func NewProductLookupSyncHandler(jobRepo job.Repository, applier lookupsync.ProductApplier, logger zerolog.Logger) *ProductLookupSyncHandler {
	return &ProductLookupSyncHandler{jobRepo: jobRepo, applier: applier, logger: logger}
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker. A
// per-child failure never nacks the delivery (returns nil).
func (h *ProductLookupSyncHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := uuid.Parse(msg.JobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}
	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	if err := exec.Start(); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product lookup sync: job state transition failed; continuing")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product lookup sync: persist PROCESSING failed")
	}

	res, runErr := h.apply(ctx, msg)
	if runErr != nil {
		h.markFailed(ctx, exec, msg, runErr)
		return nil
	}
	h.markCompleted(ctx, exec, msg, res)
	return nil
}

func (h *ProductLookupSyncHandler) apply(ctx context.Context, msg rabbitmq.JobMessage) (lookupsync.ProductApplyResult, error) {
	if h.applier == nil {
		return lookupsync.ProductApplyResult{}, errors.New("lookup sync applier not configured")
	}
	if msg.ProductSysID == 0 {
		return lookupsync.ProductApplyResult{}, errors.New("product_sys_id is required")
	}
	scope, err := lookupsyncjob.DecodeScope(msg.LookupSyncScope)
	if err != nil {
		return lookupsync.ProductApplyResult{}, err
	}
	return h.applier.ApplyProduct(ctx, msg.ProductSysID, scope, msg.CreatedBy)
}

func (h *ProductLookupSyncHandler) markCompleted(
	ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, res lookupsync.ProductApplyResult,
) {
	summary, err := json.Marshal(lookupsyncjob.ChildResult{
		ProductSysID: msg.ProductSysID, ProductCode: res.ProductCode,
		ValuesWritten: res.ValuesWritten, ValuesCleared: res.ValuesCleared,
		Skipped: res.Skipped, SkipReason: res.SkipReason,
	})
	if err != nil {
		summary = nil
	}
	if err := exec.Complete(summary); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product lookup sync: complete state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("product lookup sync: persist COMPLETED failed")
	}
	h.handleChildCompletion(ctx, exec, msg, true)
	h.logger.Info().Str("job_id", msg.JobID).Int64("product_sys_id", msg.ProductSysID).
		Int("values_written", res.ValuesWritten).Int("values_cleared", res.ValuesCleared).
		Bool("skipped", res.Skipped).Str("skip_reason", res.SkipReason).
		Msg("product lookup sync completed")
}

func (h *ProductLookupSyncHandler) markFailed(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, runErr error) {
	if err := exec.Fail(runErr.Error()); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product lookup sync: fail state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("product lookup sync: persist FAILED failed")
	}
	h.handleChildCompletion(ctx, exec, msg, false)
	h.logger.Error().Err(runErr).Str("job_id", msg.JobID).Int64("product_sys_id", msg.ProductSysID).
		Msg("product lookup sync failed")
}

func (h *ProductLookupSyncHandler) handleChildCompletion(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, success bool) {
	parentID := exec.ParentJobID()
	if parentID == nil {
		return
	}
	batchComplete, err := h.jobRepo.IncrementChildProgress(ctx, *parentID, success)
	if err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Str("parent_job_id", parentID.String()).
			Msg("product lookup sync: increment parent batch progress failed")
		return
	}
	if batchComplete {
		h.completeParentJob(ctx, *parentID)
	}
}

func (h *ProductLookupSyncHandler) completeParentJob(ctx context.Context, parentID uuid.UUID) {
	parent, err := h.jobRepo.GetByID(ctx, parentID)
	if err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("product lookup sync: load parent job for batch-complete failed")
		return
	}
	if parent.CompletedChildren() == 0 && parent.FailedChildren() > 0 {
		if err := parent.Fail(fmt.Sprintf("all %d child products failed", parent.FailedChildren())); err != nil {
			h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).Msg("product lookup sync: parent fail transition failed")
		}
	} else {
		summary := fmt.Appendf(nil, `{"total_children":%d,"completed_children":%d,"failed_children":%d}`,
			parent.TotalChildren(), parent.CompletedChildren(), parent.FailedChildren())
		if err := parent.Complete(summary); err != nil {
			h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).Msg("product lookup sync: parent complete transition failed")
		}
	}
	if err := h.jobRepo.UpdateStatus(ctx, parent); err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("product lookup sync: persist parent batch completion status failed")
	}
}
