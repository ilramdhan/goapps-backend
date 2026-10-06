package erpintegration

// job_executor.go runs erp_integration batch-step jobs on the worker
// (plan-04 P3-T4; design Part 2 §10). One job = one batch step; the job's
// subtype names the step and params carry {batch_id, preview_id?, dry_run?}.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// Job progress milestones (design §10: 10 -> 30 -> 60 -> 90 -> 100).
const (
	progressStart   = 10
	progressRead    = 30
	progressCompute = 60
	progressPersist = 90
)

// errTextMax bounds ceib_error written from a failed job.
const errTextMax = 2000

// ProgressFunc reports job progress (0-100). Nil is a no-op.
type ProgressFunc func(ctx context.Context, pct int)

func (p ProgressFunc) report(ctx context.Context, pct int) {
	if p != nil {
		p(ctx, pct)
	}
}

// JobParams is the erp_integration job params payload (design §10).
type JobParams struct {
	BatchID   int64  `json:"batch_id"`
	PreviewID string `json:"preview_id,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
	// Push only (plan-06 P5-T3): the preview totals the operator confirmed
	// and the delivery-layer RBAC outcome (finance.cost.erpintegration.push)
	// attested when the job was enqueued.
	ConfirmRowCount *int64 `json:"confirm_row_count,omitempty"`
	ConfirmSumStd   string `json:"confirm_sum_std,omitempty"`
	PushPermitted   bool   `json:"push_permitted,omitempty"`
	// ADJ execute / LOCK_BATCH only (plan-06 P5-T5): the operation, the
	// preview set hash and confirmation text the operator typed, and the
	// delivery-layer RBAC outcome attested when the job was enqueued.
	Operation      string `json:"operation,omitempty"`
	ConfirmSetHash string `json:"confirm_set_hash,omitempty"`
	ConfirmText    string `json:"confirm_text,omitempty"`
	AdjPermitted   bool   `json:"adj_permitted,omitempty"`
}

// ParseJobParams decodes and validates job params.
func ParseJobParams(raw json.RawMessage) (JobParams, error) {
	var p JobParams
	if len(raw) == 0 {
		return p, fmt.Errorf("%w: empty params", ErrInvalidJobParams)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("%w: %w", ErrInvalidJobParams, err)
	}
	if p.BatchID <= 0 {
		return p, fmt.Errorf("%w: batch_id required", ErrInvalidJobParams)
	}
	return p, nil
}

// JobExecutor dispatches erp_integration jobs to their step.
type JobExecutor struct {
	jobs         job.Repository
	runner       domain.BatchTxRunner
	load         *LoadDemandStep
	coverage     *CoverageStep
	derive       *DeriveStep                     // derive (optional; WithDerive)
	validate     *ValidateStep                   // validate (optional; WithValidate)
	backfill     *BackfillAttributesHandler      // attr_backfill (optional; job_executor_backfill.go)
	backtest     *BacktestHandler                // backtest (optional; job_executor_backtest.go)
	backtestRepo domain.BacktestReportRepository // backtest report store
	push         *PushStep                       // push (optional; WithPush)
	adj          *AdjExecuteStep                 // adj_execute (optional; job_executor_adj.go)
	lock         *LockBatchStep                  // lock_batch (optional; job_executor_adj.go)
	recon        *ReconStep                      // recon (optional; job_executor_adj.go)
	sched        *ScheduleRunner                 // scheduled chain (optional; job_executor_adj.go)
}

// NewJobExecutor wires the executor. runner is used to record ceib_error on
// a failed step (best effort, under the G11 lock).
func NewJobExecutor(jobs job.Repository, runner domain.BatchTxRunner, load *LoadDemandStep, coverage *CoverageStep) *JobExecutor {
	return &JobExecutor{jobs: jobs, runner: runner, load: load, coverage: coverage}
}

// WithDerive attaches the derive step (plan-05 P4-T4).
func (e *JobExecutor) WithDerive(d *DeriveStep) *JobExecutor {
	e.derive = d
	return e
}

// WithValidate attaches the validate step (plan-06 P5-T2).
func (e *JobExecutor) WithValidate(v *ValidateStep) *JobExecutor {
	e.validate = v
	return e
}

// WithPush attaches the W1 push step (plan-06 P5-T3).
func (e *JobExecutor) WithPush(p *PushStep) *JobExecutor {
	e.push = p
	return e
}

// Execute runs one job. A terminal job is acknowledged (redelivery). A step
// error fails the job, records ceib_error on the batch (the batch status is
// unchanged: a failed step is retryable, design §10) and is returned so the
// consumer dead-letters the message.
func (e *JobExecutor) Execute(ctx context.Context, jobID uuid.UUID) error {
	exec, err := e.jobs.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("get erp_integration job %s: %w", jobID, err)
	}
	if exec.Status().IsTerminal() {
		log.Info().Str("job_id", jobID.String()).Str("status", string(exec.Status())).
			Msg("erp_integration job already terminal; acknowledging")
		return nil
	}
	if err := exec.Start(); err != nil {
		return fmt.Errorf("start erp_integration job %s: %w", jobID, err)
	}
	if err := e.jobs.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("mark erp_integration job %s processing: %w", jobID, err)
	}
	progress := e.progressFunc(jobID)
	progress.report(ctx, progressStart)
	if exec.Subtype() == SubtypeAttrBackfill {
		return e.runAttrBackfill(ctx, exec, progress)
	}
	if exec.Subtype() == SubtypeBacktest {
		return e.runBacktest(ctx, exec, progress)
	}

	params, err := ParseJobParams(exec.Params())
	if err != nil {
		return e.failJob(ctx, exec, 0, err)
	}
	result, err := e.dispatch(ctx, exec, params, progress)
	if err != nil {
		return e.failJob(ctx, exec, params.BatchID, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return e.failJob(ctx, exec, params.BatchID, fmt.Errorf("encode result: %w", err))
	}
	if err := exec.Complete(raw); err != nil {
		return fmt.Errorf("complete erp_integration job %s: %w", jobID, err)
	}
	if err := e.jobs.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("mark erp_integration job %s success: %w", jobID, err)
	}
	e.continueSchedule(ctx, exec, params.BatchID)
	return nil
}

func (e *JobExecutor) dispatch(ctx context.Context, exec *job.Execution, p JobParams, progress ProgressFunc) (any, error) {
	actor := exec.CreatedBy()
	switch exec.Subtype() {
	case StepLoadDemand:
		if e.load == nil {
			return nil, ErrDemandReaderNotConfigured
		}
		return e.load.Run(ctx, p.BatchID, actor, progress)
	case StepCoverage:
		if e.coverage == nil {
			return nil, fmt.Errorf("%w: coverage step not configured", ErrUnknownStep)
		}
		return e.coverage.Run(ctx, p.BatchID, actor, progress)
	case StepDerive:
		if e.derive == nil {
			return nil, fmt.Errorf("%w: derive step not configured", ErrUnknownStep)
		}
		return e.derive.Run(ctx, p.BatchID, actor, progress)
	case StepValidate:
		if e.validate == nil {
			return nil, fmt.Errorf("%w: validate step not configured", ErrUnknownStep)
		}
		return e.validate.Run(ctx, p.BatchID, actor, progress)
	case StepPush:
		if e.push == nil {
			return nil, fmt.Errorf("%w: push step not configured", domain.ErrWriterNotConfigured)
		}
		return e.push.Run(ctx, PushCommand{
			BatchID: p.BatchID, Actor: actor, JobID: exec.ID().String(), HasPermission: p.PushPermitted,
			ConfirmRowCount: p.ConfirmRowCount, ConfirmSumStd: p.ConfirmSumStd,
		}, progress)
	default:
		return e.dispatchAdj(ctx, exec, p, progress)
	}
}

func (e *JobExecutor) progressFunc(jobID uuid.UUID) ProgressFunc {
	return func(ctx context.Context, pct int) {
		if err := e.jobs.UpdateProgress(ctx, jobID, pct); err != nil {
			log.Warn().Err(err).Str("job_id", jobID.String()).Int("progress", pct).
				Msg("erp_integration progress update failed")
		}
	}
}

func (e *JobExecutor) failJob(ctx context.Context, exec *job.Execution, batchID int64, cause error) error {
	if err := exec.Fail(cause.Error()); err == nil {
		if uerr := e.jobs.UpdateStatus(ctx, exec); uerr != nil {
			log.Error().Err(uerr).Str("job_id", exec.ID().String()).Msg("mark erp_integration job failed")
		}
	}
	if batchID > 0 && e.runner != nil {
		e.recordBatchError(ctx, batchID, fmt.Sprintf("%s: %s", exec.Subtype(), cause.Error()))
	}
	return cause
}

// recordBatchError writes ceib_error without changing the batch status.
func (e *JobExecutor) recordBatchError(ctx context.Context, batchID int64, text string) {
	if r := []rune(text); len(r) > errTextMax {
		text = string(r[:errTextMax])
	}
	err := e.runner.RunLocked(ctx, batchID, func(ctx context.Context, st domain.BatchStore) error {
		b, err := st.GetForUpdate(ctx)
		if err != nil {
			return err
		}
		b.SetErrorText(text)
		return st.Save(ctx, b, b.Status(), domain.Invalidation{})
	})
	if err != nil && !errors.Is(err, domain.ErrBatchNotFound) {
		log.Warn().Err(err).Int64("batch_id", batchID).Msg("record erp batch error failed")
	}
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
