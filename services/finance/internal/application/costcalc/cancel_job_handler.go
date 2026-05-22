package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// CancelJobCommand carries inputs for cancelling a running job.
type CancelJobCommand struct {
	JobID  int64
	Actor  string
	Reason string
}

// CancelJobHandler transitions a job to CANCELLED and marks any remaining
// PENDING / READY job_product rows as SKIPPED.
type CancelJobHandler struct {
	svc *Service
}

// NewCancelJobHandler constructs the handler.
func NewCancelJobHandler(svc *Service) *CancelJobHandler {
	return &CancelJobHandler{svc: svc}
}

// Handle executes the cancellation.
func (h *CancelJobHandler) Handle(ctx context.Context, cmd CancelJobCommand) (*costcalcdom.Job, error) {
	if cmd.JobID <= 0 {
		return nil, errors.New(errMsgJobIDPositive)
	}
	job, err := h.svc.jobRepo.GetByID(ctx, cmd.JobID)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	if err := job.Cancel(); err != nil {
		return nil, fmt.Errorf("cancel job: %w", err)
	}
	if err := h.svc.jobRepo.UpdateStatus(ctx, job.ID(), job.Status()); err != nil {
		return nil, fmt.Errorf("update job status: %w", err)
	}
	if err := h.svc.productRepo.MarkSkippedForJob(ctx, job.ID()); err != nil {
		return nil, fmt.Errorf("mark skipped: %w", err)
	}
	h.svc.emitAudit(ctx, AuditEvent{
		EventType:  "COST_CALC_JOB_CANCELLED",
		EntityKind: auditEntityKindJob,
		EntityID:   fmt.Sprintf("%d", job.ID()),
		Actor:      cmd.Actor,
		Message:    fmt.Sprintf("job cancelled: %s", cmd.Reason),
	})
	return job, nil
}
