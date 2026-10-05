package erpintegration

// push_execute_handler.go implements the push step (plan-06 P5-T3 step 3;
// design Part 2 §8.1 PushErpBatch, §7.2, §9.2 push_step; Part 1 §S-R11,
// §S-R12, AC-06). It writes the batch to Oracle through the W1 insert-only
// allowlist writer ONLY (W1_INSERT_BATCH + W1_INSERT_COST in one Oracle
// transaction; no UPDATE/DELETE/DDL exists on the port).
//
// Order: G1 flag → G2 writer → G3 permission (before any I/O) → under the
// G11 lock: batch gates, G10, digest + R-9 re-verification, confirmation
// match, no earlier W1 attempt → call log STARTED (committed) → InsertBatch →
// call log terminal status → batch PUSHED (or FAILED) → ERP_PUSH audit.
//
// A W1 is never retried with the same batch id: any earlier call-log row for
// the batch refuses the push with 0 writer calls. SUPERSEDE is deliberately
// NOT done here: design §5.3 / C-8 supersede the prior active batch only
// after VALUATE_ADJ succeeds (PUSHED has no SUPERSEDED edge), so the period
// keeps its active batch until the new one is valuated.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// StepPush is the erp_integration subtype and the ceib_summary key.
const StepPush = "push"

// flagPushEnabled names the G1 flag of the push.
const flagPushEnabled = "erp_integration.push_enabled"

// Push errors.
var (
	// ErrPushAlreadyAttempted is returned when a W1 call was already logged
	// for the batch (no retry with the same batch id; start a new batch or
	// resolve an UNKNOWN outcome by reconciliation).
	ErrPushAlreadyAttempted = errors.New("erpintegration: a W1 push was already attempted for this batch; create a new batch")
	// ErrCallLogNotConfigured is returned when no Oracle call log is wired
	// (§S-R11: no Oracle write without its audit row).
	ErrCallLogNotConfigured = errors.New("erpintegration: oracle call log not configured")
)

// PushCommand requests the W1 push. ConfirmRowCount / ConfirmSumStd are the
// preview totals the operator confirmed; HasPermission is the RBAC outcome.
type PushCommand struct {
	BatchID         int64
	Actor           string
	JobID           string
	HasPermission   bool
	ConfirmRowCount *int64
	ConfirmSumStd   string
}

// PushSummary is ceib_summary.push and the job result.
type PushSummary struct {
	BatchID    int64     `json:"batch_id"`
	Period     string    `json:"period"`
	Seq        int       `json:"seq"`
	CallID     string    `json:"call_id"`
	CallStatus string    `json:"call_status"`
	OraCode    string    `json:"ora_code,omitempty"`
	Error      string    `json:"error,omitempty"`
	RowCount   int64     `json:"row_count"`
	SumStd     string    `json:"sum_std"`
	RowsMD5    string    `json:"rows_md5"`
	RuleHash   string    `json:"rule_hash"`
	BatchRows  int64     `json:"batch_rows"`
	CostRows   int64     `json:"cost_rows"`
	WriterMode string    `json:"writer_mode"`
	Status     string    `json:"batch_status"`
	RunAt      time.Time `json:"run_at"`
}

// PushStep runs the W1 push.
type PushStep struct {
	runner      domain.BatchTxRunner
	writer      WriterGate
	calls       domain.OracleCallLog
	audit       AuditSink
	pushEnabled bool
	callTimeout time.Duration
	now         func() time.Time
	newCallID   func() string
}

// NewPushStep builds the step. writer comes from the fail-closed factory;
// calls must be non-nil (fail closed otherwise); audit may be nil.
// callTimeout bounds the Oracle call (<= 0: no extra bound).
func NewPushStep(runner domain.BatchTxRunner, writer WriterGate, calls domain.OracleCallLog, audit AuditSink, pushEnabled bool, callTimeout time.Duration) *PushStep {
	return &PushStep{
		runner: runner, writer: writer, calls: calls, audit: audit, pushEnabled: pushEnabled,
		callTimeout: callTimeout, now: time.Now, newCallID: uuid.NewString,
	}
}

// WithClock overrides the clock (tests).
func (s *PushStep) WithClock(now func() time.Time) *PushStep {
	s.now = now
	return s
}

// checkGates is G1, G2 and G3: no I/O at all before they pass.
func (s *PushStep) checkGates(cmd PushCommand) error {
	if err := checkFlag(s.pushEnabled, flagPushEnabled); err != nil {
		return err
	}
	if err := checkWriter(s.writer); err != nil {
		return err
	}
	if err := checkPermission(cmd.HasPermission, ErrPushPermissionDenied); err != nil {
		return err
	}
	if s.calls == nil {
		return ErrCallLogNotConfigured
	}
	return nil
}

// Run pushes the batch. A writer failure moves the batch to FAILED (the
// FAILED state commits) and the failure is returned so the job fails too.
func (s *PushStep) Run(ctx context.Context, cmd PushCommand, progress ProgressFunc) (PushSummary, error) {
	if err := s.checkGates(cmd); err != nil {
		return PushSummary{}, err
	}
	confirmSum, err := parseConfirm(cmd)
	if err != nil {
		return PushSummary{}, err
	}
	var (
		sum      PushSummary
		writeErr error
	)
	err = s.runner.RunLocked(ctx, cmd.BatchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.PushStore)
		if !ok {
			return ErrPushStoreUnsupported
		}
		var err error
		sum, writeErr, err = s.pushLocked(ctx, st, cmd, confirmSum, progress)
		return err
	})
	if err != nil {
		return PushSummary{}, err
	}
	s.emitAudit(ctx, sum, cmd.Actor)
	if writeErr != nil {
		return sum, fmt.Errorf("push batch %d: %w", cmd.BatchID, writeErr)
	}
	return sum, nil
}

func parseConfirm(cmd PushCommand) (decimal.Decimal, error) {
	if cmd.ConfirmRowCount == nil || strings.TrimSpace(cmd.ConfirmSumStd) == "" {
		return decimal.Decimal{}, fmt.Errorf("%w: confirm_row_count and confirm_sum_std are required", domain.ErrConfirmMismatch)
	}
	d, err := decimal.NewFromString(strings.TrimSpace(cmd.ConfirmSumStd))
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%w: confirm_sum_std: %w", domain.ErrConfirmMismatch, err)
	}
	return domain.Round5(d), nil
}

// pushLocked runs inside the G11 transaction. The returned writeErr is the
// Oracle failure recorded on the (committed) FAILED batch; err aborts the tx.
func (s *PushStep) pushLocked(ctx context.Context, st domain.PushStore, cmd PushCommand, confirmSum decimal.Decimal, progress ProgressFunc) (PushSummary, error, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return PushSummary{}, nil, err
	}
	prev := b.Status()
	plan, err := verifyPush(ctx, st, b)
	if err != nil {
		return PushSummary{}, nil, err
	}
	if *cmd.ConfirmRowCount != plan.digest.Totals.RowCount() || !confirmSum.Equal(plan.digest.Totals.SumStd()) {
		return PushSummary{}, nil, fmt.Errorf("%w: confirmed %d / %s, batch has %d / %s", domain.ErrConfirmMismatch,
			*cmd.ConfirmRowCount, confirmSum.StringFixed(domain.ScaleR5),
			plan.digest.Totals.RowCount(), plan.digest.Totals.SumStd().StringFixed(domain.ScaleR5))
	}
	attempted, err := s.calls.HasAttempt(ctx, b.ID(), domain.CallKeyW1InsertBatch)
	if err != nil {
		return PushSummary{}, nil, fmt.Errorf("check earlier W1 attempt: %w", err)
	}
	if attempted {
		return PushSummary{}, nil, ErrPushAlreadyAttempted
	}
	header, err := s.pushHeader(b, plan, cmd.Actor)
	if err != nil {
		return PushSummary{}, nil, err
	}
	progress.report(ctx, progressRead)
	sum := s.newSummary(b, plan)
	res, writeErr, err := s.callWriter(ctx, cmd, header, plan.rows, &sum)
	if err != nil {
		return PushSummary{}, nil, err
	}
	progress.report(ctx, progressPersist)
	if err := s.finishBatch(ctx, st, b, prev, cmd.Actor, res, writeErr, &sum); err != nil {
		return PushSummary{}, nil, err
	}
	return sum, writeErr, nil
}

func (s *PushStep) pushHeader(b *domain.Batch, plan pushPlan, actor string) (domain.PushBatch, error) {
	seq, err := seqInt32(b.Seq())
	if err != nil {
		return domain.PushBatch{}, err
	}
	t := plan.digest.Totals
	return domain.PushBatch{
		BatchID: b.ID(), Period: b.Period(), Seq: seq, RuleHash: b.RuleHash(), RowCount: t.RowCount(),
		SumStd: t.SumStd(), SumConv: t.SumConv(), SumPvl: t.SumPvl(), PushedBy: strings.TrimSpace(actor),
	}, nil
}

func (s *PushStep) newSummary(b *domain.Batch, plan pushPlan) PushSummary {
	t := plan.digest.Totals
	return PushSummary{
		BatchID: b.ID(), Period: b.Period(), Seq: b.Seq(), RowCount: t.RowCount(),
		SumStd: t.SumStd().StringFixed(domain.ScaleR5), RowsMD5: plan.digest.RowsMD5, RuleHash: b.RuleHash(),
		WriterMode: string(s.writer.Mode), RunAt: s.now(),
	}
}

// callParams are the non-secret binds logged with the call.
type callParams struct {
	Period   string `json:"period"`
	Seq      int32  `json:"seq"`
	RuleHash string `json:"rule_hash"`
	RowCount int64  `json:"row_count"`
	SumStd   string `json:"sum_std"`
	RowsMD5  string `json:"rows_md5"`
	Mode     string `json:"writer_mode"`
}

// callWriter logs STARTED (committed before the call, §S-R11), issues the
// single W1 call and logs its terminal status. err (not writeErr) means the
// call was never issued.
func (s *PushStep) callWriter(ctx context.Context, cmd PushCommand, h domain.PushBatch, rows []domain.PushRow, sum *PushSummary) (domain.WriteResult, error, error) {
	params, err := json.Marshal(callParams{Period: h.Period, Seq: h.Seq, RuleHash: h.RuleHash, RowCount: h.RowCount,
		SumStd: sum.SumStd, RowsMD5: sum.RowsMD5, Mode: string(s.writer.Mode)})
	if err != nil {
		return domain.WriteResult{}, nil, fmt.Errorf("encode call params: %w", err)
	}
	callID := s.newCallID()
	sum.CallID = callID
	if err := s.calls.Start(ctx, domain.OracleCallStart{
		CallID: callID, BatchID: h.BatchID, JobID: cmd.JobID, StatementKey: domain.CallKeyW1InsertBatch,
		Params: params, Actor: strings.TrimSpace(cmd.Actor), Attempt: 1,
	}); err != nil {
		return domain.WriteResult{}, nil, fmt.Errorf("log W1 call start: %w", err)
	}
	callCtx, cancel := s.callContext(ctx)
	res, writeErr := s.writer.Writer.InsertBatch(callCtx, h, rows)
	cancel()
	fin := classifyWrite(res, writeErr)
	sum.CallStatus, sum.OraCode, sum.Error = string(fin.Status), fin.OraCode, fin.Error
	sum.BatchRows, sum.CostRows = res.BatchRows, res.CostRows
	if err := s.calls.Finish(context.WithoutCancel(ctx), callID, fin); err != nil {
		// The Oracle outcome stands; the STARTED row is resolved to UNKNOWN
		// on worker start and by reconciliation.
		log.Error().Err(err).Str("call_id", callID).Int64("batch_id", h.BatchID).Msg("erp push: log W1 call finish failed")
	}
	return res, writeErr, nil
}

func (s *PushStep) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.callTimeout > 0 {
		return context.WithTimeout(ctx, s.callTimeout)
	}
	return context.WithCancel(ctx)
}

// classifyWrite maps the writer outcome to the call-log terminal status. An
// error that proves Oracle did not commit (application error, busy, timeout
// before commit, disabled writer) is FAILED; anything else is UNKNOWN.
func classifyWrite(res domain.WriteResult, err error) domain.OracleCallFinish {
	if err == nil {
		rows := res.CostRows
		return domain.OracleCallFinish{Status: domain.OracleCallSuccess, Rows: &rows}
	}
	fin := domain.OracleCallFinish{Status: domain.OracleCallUnknown, Error: truncateText(err.Error())}
	var app *domain.OracleAppError
	switch {
	case errors.As(err, &app):
		fin.Status, fin.OraCode = domain.OracleCallFailed, fmt.Sprintf("ORA-%05d", app.Code)
	case errors.Is(err, domain.ErrOutcomeUnknown):
	case errors.Is(err, domain.ErrOracleBusy):
		fin.Status, fin.OraCode = domain.OracleCallFailed, fmt.Sprintf("ORA-%05d", domain.OraResourceBusy)
	case errors.Is(err, domain.ErrOracleTimeout), errors.Is(err, domain.ErrWriterDisabled):
		fin.Status = domain.OracleCallFailed
	}
	return fin
}

func truncateText(s string) string {
	if r := []rune(s); len(r) > errTextMax {
		return string(r[:errTextMax])
	}
	return s
}

// finishBatch records the outcome on the batch: PUSHED on success, FAILED
// otherwise (FAILED / UNKNOWN; an UNKNOWN is resolved by reconciliation,
// never by re-pushing the same batch id).
func (s *PushStep) finishBatch(ctx context.Context, st domain.PushStore, b *domain.Batch, prev domain.BatchStatus, actor string, res domain.WriteResult, writeErr error, sum *PushSummary) error {
	at := s.now()
	if writeErr == nil {
		if res.CostRows != sum.RowCount {
			log.Warn().Int64("batch_id", b.ID()).Int64("cost_rows", res.CostRows).Int64("row_count", sum.RowCount).
				Msg("erp push: writer cost-row count differs from control total")
		}
		if _, err := b.Transition(domain.StatusPushed, actor, at); err != nil {
			return err
		}
		b.SetErrorText("")
	} else if err := b.Fail(truncateText(fmt.Sprintf("push %s: %s", sum.CallStatus, writeErr.Error())), actor, at); err != nil {
		return err
	}
	sum.Status = string(b.Status())
	if err := setStepSummary(b, StepPush, *sum); err != nil {
		return err
	}
	return st.Save(ctx, b, prev, domain.Invalidation{})
}

// emitAudit records ERP_PUSH (best effort: the outcome is committed and the
// call log already holds the Oracle-side record).
func (s *PushStep) emitAudit(ctx context.Context, sum PushSummary, actor string) {
	if s.audit == nil {
		return
	}
	after, err := json.Marshal(sum)
	if err != nil {
		log.Warn().Err(err).Int64("batch_id", sum.BatchID).Msg("erp push: marshal audit")
		return
	}
	if err := s.audit.Emit(ctx, auditdomain.NewInput{
		EntityType: auditEntityErpBatch, EntityID: sum.BatchID, Operation: auditdomain.OpErpPush,
		AfterData: string(after), UserID: strings.TrimSpace(actor),
	}); err != nil {
		log.Warn().Err(err).Int64("batch_id", sum.BatchID).Msg("erp push: audit emit failed")
	}
}
