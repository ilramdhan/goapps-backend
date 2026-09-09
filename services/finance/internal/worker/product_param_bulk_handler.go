package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/productparambulk"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// ProductParamBulkHandler processes one Bulk Edit Product Params (F4) child
// job: it decodes the shared operations list carried on msg.Operations,
// applies every operation to the single cpm_product_sys_id in
// msg.ProductSysID via cpp.Repository.ApplyBulkOperations (one transaction
// per product — a failure here never touches any other child in the batch),
// then reports the outcome to the parent batch job via
// job.Repository.IncrementChildProgress. Mirrors MBBulkTransitionHandler's
// lifecycle exactly — see that file's doc comment for the precedent.
//
// 🔴 Registered SEQUENTIALLY in cmd/worker/main.go (rabbitmq.NewConsumer, not
// NewConcurrentConsumer), matching MBBulkTransitionHandler's registration —
// ApplyBulkOperations has not been audited for concurrent-write safety across
// children of the same batch (e.g. two children referencing the same
// lookup_fill_group_code trigger param).
type ProductParamBulkHandler struct {
	jobRepo job.Repository
	repo    cpp.Repository
	logger  zerolog.Logger
}

// NewProductParamBulkHandler constructs the handler.
func NewProductParamBulkHandler(jobRepo job.Repository, repo cpp.Repository, logger zerolog.Logger) *ProductParamBulkHandler {
	return &ProductParamBulkHandler{jobRepo: jobRepo, repo: repo, logger: logger}
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker.
//
// Lifecycle: PROCESSING -> (success: COMPLETED) | (failure: FAILED), then in
// both cases IncrementChildProgress reports the outcome to the parent batch
// job. A per-child failure never fails the whole batch and never nacks the
// delivery (returns nil).
func (h *ProductParamBulkHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := uuid.Parse(msg.JobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}

	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	if err := exec.Start(); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product param bulk: job state transition failed; continuing")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product param bulk: persist PROCESSING failed")
	}

	outcomes, runErr := h.runOperations(ctx, msg)
	if runErr != nil {
		h.markFailed(ctx, exec, msg, runErr)
		return nil
	}

	h.markCompleted(ctx, exec, msg, outcomes)
	return nil
}

// runOperations decodes msg.Operations (JSON-encoded []productparambulk.OperationDTO,
// shared identically across every child in the batch) into []cpp.BulkOp and
// applies them to msg.ProductSysID inside one repository-owned transaction.
func (h *ProductParamBulkHandler) runOperations(ctx context.Context, msg rabbitmq.JobMessage) ([]cpp.BulkOpOutcome, error) {
	var dtos []productparambulk.OperationDTO
	if err := json.Unmarshal([]byte(msg.Operations), &dtos); err != nil {
		return nil, fmt.Errorf("decode operations: %w", err)
	}
	ops := make([]cpp.BulkOp, 0, len(dtos))
	for _, dto := range dtos {
		op, err := toBulkOp(dto)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return h.repo.ApplyBulkOperations(ctx, msg.ProductSysID, ops, msg.CreatedBy, msg.SkipMissingApplicable)
}

// toBulkOp converts one transport DTO into the domain-layer operation shape.
func toBulkOp(dto productparambulk.OperationDTO) (cpp.BulkOp, error) {
	paramID, err := uuid.Parse(dto.ParamID)
	if err != nil {
		return cpp.BulkOp{}, fmt.Errorf("invalid param_id %q: %w", dto.ParamID, err)
	}
	kind, err := toBulkOpKind(dto.Kind)
	if err != nil {
		return cpp.BulkOp{}, err
	}
	return cpp.BulkOp{
		Kind:         kind,
		ParamID:      paramID,
		IsRequired:   dto.IsRequired,
		DisplayOrder: dto.DisplayOrder,
		ValueNumeric: dto.ValueNumeric,
		ValueText:    dto.ValueText,
		ValueFlag:    dto.ValueFlag,
	}, nil
}

// toBulkOpKind maps the transport-layer OpKind to the domain BulkOpKind.
func toBulkOpKind(kind productparambulk.OpKind) (cpp.BulkOpKind, error) {
	switch kind {
	case productparambulk.OpAddApplicable:
		return cpp.BulkOpAddApplicable, nil
	case productparambulk.OpRemoveApplicable:
		return cpp.BulkOpRemoveApplicable, nil
	case productparambulk.OpUpsertValue:
		return cpp.BulkOpUpsertValue, nil
	default:
		return "", fmt.Errorf("unknown bulk op kind %q", kind)
	}
}

// markCompleted persists the COMPLETED status — with any skipped operations
// (skip_missing_applicable=true, param not yet CAPP-applicable to this
// product) folded into the child job's result summary, since a skip is not a
// hard failure and therefore never appears in ListBulkProductParamJobFailures
// — and reports success to the parent batch job's progress counters.
func (h *ProductParamBulkHandler) markCompleted(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, outcomes []cpp.BulkOpOutcome) {
	summary := summarizeOutcomes(outcomes)
	if err := exec.Complete(summary); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product param bulk: complete state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("product param bulk: persist COMPLETED failed")
	}
	h.handleChildCompletion(ctx, exec, msg, true)
	h.logger.Info().Str("job_id", msg.JobID).Int64("product_sys_id", msg.ProductSysID).
		Msg("product param bulk completed")
}

// summarizeOutcomes JSON-encodes the per-op outcomes for storage as the
// child job's result_summary. A marshal failure is logged and swallowed —
// summary is best-effort context, never load-bearing for job status.
func summarizeOutcomes(outcomes []cpp.BulkOpOutcome) json.RawMessage {
	if len(outcomes) == 0 {
		return nil
	}
	b, err := json.Marshal(map[string]any{"outcomes": outcomes})
	if err != nil {
		return nil
	}
	return b
}

// markFailed persists the FAILED status (with runErr recorded as the job's
// error_message — this IS the per-child failure detail surfaced later by
// ListBulkProductParamJobFailures, which reads it straight off the child job
// row) and reports the failure to the parent batch job's progress counters.
func (h *ProductParamBulkHandler) markFailed(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, runErr error) {
	if err := exec.Fail(runErr.Error()); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("product param bulk: fail state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("product param bulk: persist FAILED failed")
	}
	h.handleChildCompletion(ctx, exec, msg, false)
	h.logger.Error().Err(runErr).Str("job_id", msg.JobID).Int64("product_sys_id", msg.ProductSysID).
		Msg("product param bulk failed")
}

// handleChildCompletion atomically increments the parent batch job's
// completed/failed counter for one finished child and, when that increment
// reports the batch is now fully done, marks the parent job_execution row
// COMPLETED/FAILED. Mirrors MBBulkTransitionHandler.handleChildCompletion.
func (h *ProductParamBulkHandler) handleChildCompletion(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, success bool) {
	parentID := exec.ParentJobID()
	if parentID == nil {
		return
	}
	batchComplete, err := h.jobRepo.IncrementChildProgress(ctx, *parentID, success)
	if err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Str("parent_job_id", parentID.String()).
			Msg("product param bulk: increment parent batch progress failed")
		return
	}
	if !batchComplete {
		return
	}
	h.completeParentJob(ctx, *parentID)
}

// completeParentJob loads the now-fully-finished parent job and transitions
// it to a terminal status, recording the final child tallies as its result
// summary. A parent has no operations of its own to run — it exists purely
// to track the batch.
func (h *ProductParamBulkHandler) completeParentJob(ctx context.Context, parentID uuid.UUID) {
	parent, err := h.jobRepo.GetByID(ctx, parentID)
	if err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("product param bulk: load parent job for batch-complete failed")
		return
	}

	allFailed := parent.CompletedChildren() == 0 && parent.FailedChildren() > 0
	summary := fmt.Appendf(nil, `{"total_children":%d,"completed_children":%d,"failed_children":%d}`,
		parent.TotalChildren(), parent.CompletedChildren(), parent.FailedChildren())

	if allFailed {
		if err := parent.Fail(fmt.Sprintf("all %d child products failed", parent.FailedChildren())); err != nil {
			h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).
				Msg("product param bulk: parent fail state transition failed")
		}
	} else if err := parent.Complete(summary); err != nil {
		h.logger.Warn().Err(err).Str("parent_job_id", parentID.String()).
			Msg("product param bulk: parent complete state transition failed")
	}

	if err := h.jobRepo.UpdateStatus(ctx, parent); err != nil {
		h.logger.Error().Err(err).Str("parent_job_id", parentID.String()).
			Msg("product param bulk: persist parent batch completion status failed")
	}
}
