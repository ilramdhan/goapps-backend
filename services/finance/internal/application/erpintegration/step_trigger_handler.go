package erpintegration

// step_trigger_handler.go enqueues an erp_integration batch-step job
// (plan-04 P3-T4). The RPC surface (P6) returns the job id; callers poll
// GetJob.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// erpJobPriority is the queue priority of batch-step jobs.
const erpJobPriority = 5

// StepTriggerCommand requests one batch step.
type StepTriggerCommand struct {
	BatchID int64
	Step    string
	Actor   string
}

// StepTriggerHandler validates and enqueues a batch step.
type StepTriggerHandler struct {
	jobs      job.Repository
	batches   domain.BatchRepository
	publisher ErpJobPublisher
	maxAge    time.Duration
	now       func() time.Time
	// push gates (G1/G2) checked before a push job is enqueued (WithPushGates).
	pushEnabled bool
	pushWriter  WriterGate
	// ADJ gates (G1/G2) checked before a W2 job is enqueued (WithAdjGates).
	valuationEnabled  bool
	adjApproveEnabled bool
	adjWriter         WriterGate
}

// NewStepTriggerHandler builds the handler. publisher may be nil (the
// trigger then fails with ErrPublisherUnavailable). maxAge is the demand
// staleness bound enforced before coverage (<= 0 disables).
func NewStepTriggerHandler(jobs job.Repository, batches domain.BatchRepository, publisher ErpJobPublisher, maxAge time.Duration) *StepTriggerHandler {
	return &StepTriggerHandler{jobs: jobs, batches: batches, publisher: publisher, maxAge: maxAge, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *StepTriggerHandler) WithClock(now func() time.Time) *StepTriggerHandler {
	h.now = now
	return h
}

// WithPushGates sets the G1 flag and the G2 writer the push trigger checks
// up front (the push step re-checks both on the worker). Unset: push is
// refused (fail closed).
func (h *StepTriggerHandler) WithPushGates(pushEnabled bool, writer WriterGate) *StepTriggerHandler {
	h.pushEnabled, h.pushWriter = pushEnabled, writer
	return h
}

// PushTriggerCommand requests the W1 push of a previewed batch.
type PushTriggerCommand struct {
	BatchID         int64
	Actor           string
	HasPermission   bool
	ConfirmRowCount int64
	ConfirmSumStd   string
}

// TriggerPush checks G1, G2, G3 and the batch gates, then enqueues the push
// job carrying the confirmed preview totals. The worker re-verifies
// everything (G10, digest, R-9, confirmation) under the G11 lock before the
// single W1 call.
func (h *StepTriggerHandler) TriggerPush(ctx context.Context, cmd PushTriggerCommand) (*job.Execution, error) {
	if err := checkFlag(h.pushEnabled, flagPushEnabled); err != nil {
		return nil, err
	}
	if err := checkWriter(h.pushWriter); err != nil {
		return nil, err
	}
	if err := checkPermission(cmd.HasPermission, ErrPushPermissionDenied); err != nil {
		return nil, err
	}
	if _, err := parseConfirm(PushCommand{ConfirmRowCount: &cmd.ConfirmRowCount, ConfirmSumStd: cmd.ConfirmSumStd}); err != nil {
		return nil, err
	}
	n := cmd.ConfirmRowCount
	return h.enqueue(ctx, StepTriggerCommand{BatchID: cmd.BatchID, Step: StepPush, Actor: cmd.Actor}, JobParams{
		ConfirmRowCount: &n, ConfirmSumStd: strings.TrimSpace(cmd.ConfirmSumStd), PushPermitted: true,
	})
}

// Handle checks the step is allowed in the batch's current status, creates
// a QUEUED job (one active erp_integration job per period), links it to the
// batch and publishes it.
func (h *StepTriggerHandler) Handle(ctx context.Context, cmd StepTriggerCommand) (*job.Execution, error) {
	switch cmd.Step {
	case StepPush:
		// Push needs the confirmed preview totals and G1-G3: TriggerPush.
		return nil, fmt.Errorf("%w: use TriggerPush", ErrStepNotAllowed)
	case StepAdjExecute, StepLockBatch:
		// W2 package calls need G1-G5: TriggerAdjExecute / TriggerLockBatch.
		return nil, fmt.Errorf("%w: use TriggerAdjExecute / TriggerLockBatch", ErrStepNotAllowed)
	}
	return h.enqueue(ctx, cmd, JobParams{})
}

// enqueue is the shared trigger path; params carries step-specific fields
// (BatchID is set here).
func (h *StepTriggerHandler) enqueue(ctx context.Context, cmd StepTriggerCommand, params JobParams) (*job.Execution, error) {
	if h.publisher == nil {
		return nil, ErrPublisherUnavailable
	}
	actor := strings.TrimSpace(cmd.Actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: actor required", ErrInvalidJobParams)
	}
	b, err := h.batches.GetByID(ctx, cmd.BatchID)
	if err != nil {
		return nil, err
	}
	if err := h.checkStep(b, cmd.Step); err != nil {
		return nil, err
	}
	active, err := h.jobs.HasActiveJob(ctx, job.TypeErpIntegration, b.Period())
	if err != nil {
		return nil, fmt.Errorf("check active erp_integration job: %w", err)
	}
	if active {
		return nil, job.ErrDuplicateActiveJob
	}
	params.BatchID = b.ID()
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}
	exec, err := job.NewExecution(job.TypeErpIntegration, cmd.Step, b.Period(), actor, erpJobPriority, raw)
	if err != nil {
		return nil, fmt.Errorf("new erp_integration job: %w", err)
	}
	if err := h.jobs.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist erp_integration job: %w", err)
	}
	b.SetJobID(exec.ID().String())
	if err := h.batches.Save(ctx, b, b.Status(), domain.Invalidation{}); err != nil {
		return nil, h.abort(ctx, exec, fmt.Errorf("link job to batch: %w", err))
	}
	if err := h.publisher.PublishErpIntegration(ctx, exec.ID().String(), cmd.Step, b.Period(), actor); err != nil {
		return nil, h.abort(ctx, exec, fmt.Errorf("publish erp_integration job: %w", err))
	}
	return exec, nil
}

func (h *StepTriggerHandler) checkStep(b *domain.Batch, step string) error {
	switch step {
	case StepLoadDemand:
		if !domain.CanTransition(b.Status(), domain.StatusDemandLoaded) {
			return fmt.Errorf("%w: load_demand in %s", ErrStepNotAllowed, b.Status())
		}
		return nil
	case StepCoverage:
		if b.Status() != domain.StatusDemandLoaded && b.Status() != domain.StatusCovered {
			return fmt.Errorf("%w: coverage in %s", ErrStepNotAllowed, b.Status())
		}
		return domain.CheckDemandFresh(b, h.now(), h.maxAge)
	case StepDerive:
		// The G10 period lock is re-checked by the step inside the locked tx.
		if err := checkDeriveStatus(b); err != nil {
			return err
		}
		return domain.CheckDemandFresh(b, h.now(), h.maxAge)
	case StepValidate:
		// Demand freshness is V-10 inside the step (a finding that revokes a
		// stale VALIDATED batch), and G10 is checked there under the lock.
		return checkValidateStatus(b)
	case StepPush:
		// G10, the digest and R-9 are re-checked by the step under the lock.
		return checkPushBatch(b)
	case StepRecon:
		// Read-only towards Oracle: no write gate; re-checked under the lock.
		return checkReconBatch(b)
	default:
		return checkAdjStep(b, step)
	}
}

func (h *StepTriggerHandler) abort(ctx context.Context, exec *job.Execution, cause error) error {
	if err := exec.Fail(cause.Error()); err == nil {
		if uerr := h.jobs.UpdateStatus(ctx, exec); uerr != nil {
			log.Error().Err(uerr).Str("job_id", exec.ID().String()).Msg("mark aborted erp_integration job failed")
		}
	}
	return cause
}
