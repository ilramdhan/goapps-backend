package erpintegration

// lock_batch_handler.go implements the LOCK_BATCH step (plan-06 P5-T5;
// design Part 2 §9.4; §5.3 RECONCILED → LOCKED). It runs only after the
// period's ADJ heads are posted in Oracle (read-only probe) on a RECONCILED
// LIVE batch, under the G11 lock with G10, and changes Oracle only through
// the OracleWriter port (PKG_GOAPPS_ADJ.LOCK_BATCH). ORA-20904 (the package
// refusing a not-yet-posted period) maps to ErrAdjNotPosted. No preview is
// needed: the call changes only the GoApps batch header in Oracle.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// StepLockBatch is the erp_integration subtype of the LOCK_BATCH step.
const StepLockBatch = "lock_batch"

// ErrAdjNotPosted is returned when LOCK_BATCH runs before the period's ADJ
// heads are posted (probe or ORA-20904).
var ErrAdjNotPosted = errors.New("erpintegration: LOCK_BATCH requires the period's ADJ heads to be posted")

// LockBatchCommand requests LOCK_BATCH.
type LockBatchCommand struct {
	BatchID       int64
	Actor         string
	JobID         string
	HasPermission bool
}

// LockBatchSummary is ceib_summary.lock_batch and the job result.
type LockBatchSummary struct {
	BatchID       int64     `json:"batch_id"`
	Period        string    `json:"period"`
	PostedHeads   int64     `json:"posted_heads"`
	CallID        string    `json:"call_id,omitempty"`
	CallStatus    string    `json:"call_status,omitempty"`
	Attempts      int       `json:"attempts"`
	OraCode       string    `json:"ora_code,omitempty"`
	Error         string    `json:"error,omitempty"`
	OracleSummary string    `json:"oracle_summary,omitempty"`
	AlreadyDone   bool      `json:"already_done"`
	WriterMode    string    `json:"writer_mode"`
	Status        string    `json:"batch_status"`
	RunAt         time.Time `json:"run_at"`
}

// LockBatchStep runs LOCK_BATCH.
type LockBatchStep struct {
	runner  domain.BatchTxRunner
	writer  WriterGate
	calls   domain.OracleCallLog
	audit   AuditSink
	prober  domain.ErpAdjHeadProber
	enabled bool
	caller  *w2Caller
	now     func() time.Time
}

// NewLockBatchStep builds the step. enabled is the G1 flag
// (erp_integration.valuation_enabled); prober nil fails closed.
func NewLockBatchStep(runner domain.BatchTxRunner, writer WriterGate, calls domain.OracleCallLog, audit AuditSink,
	prober domain.ErpAdjHeadProber, enabled bool, callTimeout time.Duration, busyRetries int,
) *LockBatchStep {
	return &LockBatchStep{
		runner: runner, writer: writer, calls: calls, audit: audit, prober: prober, enabled: enabled,
		caller: newW2Caller(writer, calls, callTimeout, busyRetries), now: time.Now,
	}
}

// WithClock overrides the clock (tests).
func (s *LockBatchStep) WithClock(now func() time.Time) *LockBatchStep {
	s.now = now
	return s
}

// WithSleep overrides the busy backoff wait (tests).
func (s *LockBatchStep) WithSleep(sleep func(ctx context.Context, d time.Duration) error) *LockBatchStep {
	s.caller.sleep = sleep
	return s
}

func (s *LockBatchStep) checkGates(cmd LockBatchCommand) error {
	if err := checkPermission(cmd.HasPermission, ErrAdjPermissionDenied); err != nil {
		return err
	}
	if strings.TrimSpace(cmd.Actor) == "" {
		return domain.ErrActorRequired
	}
	if err := checkFlag(s.enabled, flagValuationEnabled); err != nil {
		return err
	}
	if err := checkWriter(s.writer); err != nil {
		return err
	}
	switch {
	case s.calls == nil:
		return ErrCallLogNotConfigured
	case s.prober == nil:
		return ErrPostedProbeNotConfigured
	}
	return nil
}

// Run locks the batch. A failed call keeps the batch RECONCILED (error text
// commits) and is returned so the job fails.
func (s *LockBatchStep) Run(ctx context.Context, cmd LockBatchCommand, progress ProgressFunc) (LockBatchSummary, error) {
	if err := s.checkGates(cmd); err != nil {
		return LockBatchSummary{}, err
	}
	var (
		sum     LockBatchSummary
		callErr error
	)
	err := s.runner.RunLocked(ctx, cmd.BatchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.AdjExecStore)
		if !ok {
			return ErrAdjStoreUnsupported
		}
		var err error
		sum, callErr, err = s.lockLocked(ctx, st, cmd, progress)
		return err
	})
	if err != nil {
		return LockBatchSummary{}, err
	}
	emitBatchAudit(ctx, s.audit, auditdomain.OpErpBatchLock, sum.BatchID, sum, cmd.Actor)
	if callErr != nil {
		return sum, fmt.Errorf("%s batch %d: %w", domain.CallKeyW2LockBatch, cmd.BatchID, callErr)
	}
	return sum, nil
}

func (s *LockBatchStep) lockLocked(ctx context.Context, st domain.AdjExecStore, cmd LockBatchCommand, progress ProgressFunc) (LockBatchSummary, error, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return LockBatchSummary{}, nil, err
	}
	if err := checkLockBatch(ctx, st, b); err != nil {
		return LockBatchSummary{}, nil, err
	}
	posted, err := s.prober.ProbePosted(ctx, b.Period())
	if err != nil {
		return LockBatchSummary{}, nil, fmt.Errorf("probe posted ADJ heads: %w", err)
	}
	if posted <= 0 {
		return LockBatchSummary{}, nil, fmt.Errorf("%w: period %s", ErrAdjNotPosted, b.Period())
	}
	progress.report(ctx, progressRead)
	sum := LockBatchSummary{BatchID: b.ID(), Period: b.Period(), PostedHeads: posted, WriterMode: string(s.writer.Mode), RunAt: s.now()}
	out, err := s.caller.run(ctx, w2Call{
		key: domain.CallKeyW2LockBatch, batchID: b.ID(), jobID: cmd.JobID, actor: cmd.Actor,
		params: map[string]any{"batch_id": b.ID(), "period": b.Period(), "writer_mode": string(s.writer.Mode)},
		fn: func(ctx context.Context, w domain.OracleWriter) (domain.Summary, error) {
			return w.LockBatch(ctx, b.ID())
		},
	})
	if err != nil {
		return LockBatchSummary{}, nil, err
	}
	sum.CallID, sum.CallStatus, sum.Attempts = out.callID, string(out.fin.Status), out.attempts
	sum.OraCode, sum.Error, sum.OracleSummary = out.fin.OraCode, out.fin.Error, truncateText(out.summary.Text)
	sum.AlreadyDone = out.alreadyDone()
	progress.report(ctx, progressPersist)
	prev := b.Status()
	if callErr := lockCallError(out); callErr != nil {
		b.SetErrorText(truncateText(fmt.Sprintf("%s: %s", domain.CallKeyW2LockBatch, callErr.Error())))
		sum.Status = string(b.Status())
		if err := setStepSummary(b, StepLockBatch, sum); err != nil {
			return LockBatchSummary{}, nil, err
		}
		return sum, callErr, st.Save(ctx, b, prev, domain.Invalidation{})
	}
	if _, err := b.Transition(domain.StatusLocked, cmd.Actor, s.now()); err != nil {
		return LockBatchSummary{}, nil, err
	}
	b.SetErrorText("")
	sum.Status = string(b.Status())
	if err := setStepSummary(b, StepLockBatch, sum); err != nil {
		return LockBatchSummary{}, nil, err
	}
	return sum, nil, st.Save(ctx, b, prev, domain.Invalidation{})
}

// checkLockBatch: LIVE, RECONCILED, not needs-repush, G10.
func checkLockBatch(ctx context.Context, st domain.AdjExecStore, b *domain.Batch) error {
	if b.Mode() == domain.ModeShadow {
		return fmt.Errorf("%w: batch %d", domain.ErrShadowNotPushable, b.ID())
	}
	if b.NeedsRepush() {
		return domain.ErrNeedsRepush
	}
	if err := requireStatus(b, "LOCK_BATCH", domain.StatusReconciled); err != nil {
		return err
	}
	return checkPeriodLocked(ctx, st, b.Period())
}

// lockCallError maps the LOCK_BATCH outcome. LOCK_BATCH only changes the
// GoApps batch header, so an UNKNOWN outcome is not finalized: it is held
// and a re-run (idempotent, ALREADY_DONE) resolves it.
func lockCallError(out w2Outcome) error {
	switch {
	case out.fin.Status == domain.OracleCallSuccess:
		return nil
	case oraCodeIs(out.writeErr, domain.OraCode20904):
		return fmt.Errorf("%w: %w", ErrAdjNotPosted, out.writeErr)
	case out.fin.Status == domain.OracleCallUnknown:
		return fmt.Errorf("%w: LOCK_BATCH outcome UNKNOWN; re-run to resolve (idempotent): %w", ErrAdjCallFailed, out.writeErr)
	default:
		return fmt.Errorf("%w: %s %s: %w", ErrAdjCallFailed, out.fin.Status, out.fin.OraCode, out.writeErr)
	}
}
