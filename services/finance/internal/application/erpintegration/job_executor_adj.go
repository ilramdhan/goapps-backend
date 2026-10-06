package erpintegration

// job_executor_adj.go wires the ADJ execute, LOCK_BATCH and recon steps and the
// scheduled-chain continuation into the job executor (plan-06 P5-T5,
// P5-T11). The W2 steps re-check every gate on the worker; the job params
// only carry what the operator confirmed and the RBAC attestation.

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// WithAdjExecute attaches the ADJ execute step (VALUATE/APPROVE/RESTORE).
func (e *JobExecutor) WithAdjExecute(s *AdjExecuteStep) *JobExecutor {
	e.adj = s
	return e
}

// WithLockBatch attaches the LOCK_BATCH step.
func (e *JobExecutor) WithLockBatch(s *LockBatchStep) *JobExecutor {
	e.lock = s
	return e
}

// WithRecon attaches the recon read-back step (plan-06 P5-T6).
func (e *JobExecutor) WithRecon(s *ReconStep) *JobExecutor {
	e.recon = s
	return e
}

// WithScheduleRunner attaches the scheduled-run continuation: after a
// scheduled job succeeds, the next read/compute step is enqueued.
func (e *JobExecutor) WithScheduleRunner(r *ScheduleRunner) *JobExecutor {
	e.sched = r
	return e
}

// dispatchAdj runs the W2 steps; any other subtype is unknown.
func (e *JobExecutor) dispatchAdj(ctx context.Context, exec *job.Execution, p JobParams, progress ProgressFunc) (any, error) {
	actor := exec.CreatedBy()
	switch exec.Subtype() {
	case StepAdjExecute:
		if e.adj == nil {
			return nil, fmt.Errorf("%w: adj_execute step not configured", domain.ErrWriterNotConfigured)
		}
		return e.adj.Run(ctx, AdjExecuteCommand{
			BatchID: p.BatchID, PreviewID: p.PreviewID, Operation: domain.AdjOperation(p.Operation),
			ConfirmSetHash: p.ConfirmSetHash, ConfirmText: p.ConfirmText, Actor: actor,
			JobID: exec.ID().String(), HasPermission: p.AdjPermitted,
		}, progress)
	case StepLockBatch:
		if e.lock == nil {
			return nil, fmt.Errorf("%w: lock_batch step not configured", domain.ErrWriterNotConfigured)
		}
		return e.lock.Run(ctx, LockBatchCommand{
			BatchID: p.BatchID, Actor: actor, JobID: exec.ID().String(), HasPermission: p.AdjPermitted,
		}, progress)
	case StepRecon:
		if e.recon == nil {
			return nil, ErrReconReaderNotConfigured
		}
		return e.recon.Run(ctx, p.BatchID, actor, progress)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownStep, exec.Subtype())
	}
}

// continueSchedule enqueues the next scheduled read/compute step after a
// scheduled job succeeded (best effort: a failure is logged, the completed
// job stays SUCCESS and the operator can continue by hand).
func (e *JobExecutor) continueSchedule(ctx context.Context, exec *job.Execution, batchID int64) {
	if e.sched == nil || batchID <= 0 || !IsScheduledRun(exec) {
		return
	}
	if _, err := e.sched.ContinueAfter(ctx, batchID, exec.Subtype()); err != nil {
		log.Warn().Err(err).Int64("batch_id", batchID).Str("step", exec.Subtype()).
			Msg("erp scheduled run: continue failed")
	}
}
