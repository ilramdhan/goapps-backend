package erpintegration

// valuate_handler.go implements the ADJ execute step (plan-06 P5-T5; design
// Part 2 §9.4; Part 1 §3.2 G1-G11, §S-R11, §S-T6, AC-07, AC-08). One step
// runs VALUATE_ADJ, APPROVE_ADJ or RESTORE_ADJ; the operation-specific
// parts (eligible status, the set the call changes, the observable effect
// and the PG finalization) live in approve_adj_handler.go and
// restore_adj_handler.go.
//
// Oracle is changed ONLY through the OracleWriter port (one PKG_GOAPPS_ADJ
// package call per attempt, period-wide, P_BATCH_ID only); everything else
// here reads Oracle through the read-only snapshot reader.
//
// Order: G3 permission → actor → G1 flag → G2 writer → wiring (no I/O) →
// preview G4 (OPEN, not expired, same batch and operation) → G5 confirmation
// (set hash + typed text) → G11 lock → batch status → G10 → preview
// OPEN→CONSUMED (committed; a redelivery can never call Oracle twice) → G6
// re-probe (posted/approved heads refuse; set hash compare, ALREADY_DONE
// when the effect is already visible) → G7a V-07 → call log STARTED → one
// package call (busy: bounded retry) → call log terminal → UNKNOWN: read-only
// re-probe → one PG tx {prior active → SUPERSEDED; batch → VALUATED; ...}
// → audit. Every refusal before the call makes 0 writer calls.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration/validation"
)

// StepAdjExecute is the erp_integration subtype of the ADJ execute step.
const StepAdjExecute = "adj_execute"

// Flags of the ADJ operations (G1).
const (
	flagValuationEnabled  = "erp_integration.valuation_enabled"
	flagAdjApproveEnabled = "erp_integration.adj_approve_enabled"
)

// ADJ execute errors.
var (
	// ErrAdjPermissionDenied is returned when the caller lacks the RBAC
	// permission of an ADJ operation other than VALUATE.
	ErrAdjPermissionDenied = errors.New("erpintegration: permission denied for the ADJ operation")
	// ErrAdjStoreUnsupported is returned when the batch store cannot serve
	// the ADJ execute step.
	ErrAdjStoreUnsupported = errors.New("erpintegration: batch store does not support the ADJ execute step")
	// ErrAdjOperationUnknown is returned for an operation outside
	// VALUATE / APPROVE / RESTORE.
	ErrAdjOperationUnknown = errors.New("erpintegration: unknown ADJ operation")
	// ErrAdjIneligible is returned when a head of the set is approved (or
	// the set is otherwise not eligible); no Oracle call is made.
	ErrAdjIneligible = errors.New("erpintegration: ADJ set is not eligible")
	// ErrAdjCallFailed is returned when the package call failed (FAILED or
	// busy after the retries); the batch keeps its status.
	ErrAdjCallFailed = errors.New("erpintegration: ADJ package call failed")
	// ErrAdjUnknownPartial is returned when an UNKNOWN outcome re-probe
	// finds the effect on only part of the set: the batch is held for manual
	// resolution and an alert is logged.
	ErrAdjUnknownPartial = errors.New("erpintegration: ADJ call outcome UNKNOWN_PARTIAL; batch held for manual resolution")
	// ErrNotLatestBatch is returned when RESTORE_ADJ targets a batch that is
	// not the latest of its period (C-9).
	ErrNotLatestBatch = errors.New("erpintegration: RESTORE_ADJ runs on the latest batch of the period only")
)

// PreviewGetter reads a stored preview (ValuationPreviewRepository.Get).
type PreviewGetter interface {
	Get(ctx context.Context, id string) (domain.ValuationPreview, error)
}

// AdjExecuteConfig holds the G1 flags and the call bounds.
type AdjExecuteConfig struct {
	ValuationEnabled  bool
	AdjApproveEnabled bool
	// CallTimeout bounds one package call (erp.call_timeout, default 120s).
	CallTimeout time.Duration
	// BusyRetries bounds the ORA-00054/30006 retries (erp.busy_retries, 3).
	BusyRetries int
}

// AdjExecuteCommand requests one ADJ operation against a consumed preview.
type AdjExecuteCommand struct {
	BatchID        int64
	PreviewID      string
	Operation      domain.AdjOperation
	ConfirmSetHash string
	ConfirmText    string
	Actor          string
	JobID          string
	// HasPermission is the delivery-layer RBAC outcome attested at enqueue.
	HasPermission bool
}

// AdjExecuteSummary is ceib_summary.<op> and the job result.
type AdjExecuteSummary struct {
	BatchID         int64              `json:"batch_id"`
	Period          string             `json:"period"`
	Operation       string             `json:"operation"`
	PreviewID       string             `json:"preview_id"`
	Heads           int                `json:"heads"`
	Items           int                `json:"items"`
	CallID          string             `json:"call_id,omitempty"`
	CallStatus      string             `json:"call_status,omitempty"`
	Attempts        int                `json:"attempts"`
	OraCode         string             `json:"ora_code,omitempty"`
	Error           string             `json:"error,omitempty"`
	OracleSummary   string             `json:"oracle_summary,omitempty"`
	AlreadyDone     bool               `json:"already_done"`
	Resolution      *UnknownResolution `json:"unknown_resolution,omitempty"`
	SupersededBatch int64              `json:"superseded_batch_id,omitempty"`
	RestoreVerified *bool              `json:"restore_verified,omitempty"`
	WriterMode      string             `json:"writer_mode"`
	Status          string             `json:"batch_status"`
	RunAt           time.Time          `json:"run_at"`
}

// adjSet is the set of ADJ items an operation changes. rows feed the set
// hash (G6) and expected lists the items whose effect the probe checks.
type adjSet struct {
	rows     []domain.AdjSnapshotRow
	expected []int64
	heads    int
}

// adjOpSpec is the operation-specific half of the execute flow.
type adjOpSpec struct {
	op         domain.AdjOperation
	callKey    string
	summaryKey string
	auditOp    string
	validate   bool
	// checkBatch checks the batch status for the operation.
	checkBatch func(ctx context.Context, st domain.AdjExecStore, b *domain.Batch) error
	// selectSet builds the set from the re-read rows (eligibility refusal).
	selectSet func(rows, eligible []domain.AdjSnapshotRow, std []domain.StdRow, batchID int64) (adjSet, error)
	// done reports the observable effect of the call on one row.
	done func(batchID int64) rowDoneFunc
	// alreadyDone reports the effect is already visible on the period.
	alreadyDone func(rows, eligible []domain.AdjSnapshotRow, std []domain.StdRow, batchID int64) bool
	// call issues the package call.
	call func(batchID int64, actor string) func(ctx context.Context, w domain.OracleWriter) (domain.Summary, error)
	// finalize records the success on the batch (one PG tx).
	finalize func(s *AdjExecuteStep, ctx context.Context, st domain.AdjExecStore, b *domain.Batch, actor string, sum *AdjExecuteSummary) error
}

// AdjExecuteStep runs VALUATE_ADJ / APPROVE_ADJ / RESTORE_ADJ.
type AdjExecuteStep struct {
	runner   domain.BatchTxRunner
	writer   WriterGate
	calls    domain.OracleCallLog
	audit    AuditSink
	previews PreviewGetter
	consumer domain.PreviewConsumer
	reader   domain.AdjSnapshotReader
	cfg      AdjExecuteConfig
	caller   *w2Caller
	now      func() time.Time
}

// NewAdjExecuteStep builds the step. writer comes from the fail-closed
// factory; calls, previews, consumer and reader must be non-nil (the step
// fails closed otherwise); audit may be nil.
func NewAdjExecuteStep(runner domain.BatchTxRunner, writer WriterGate, calls domain.OracleCallLog, audit AuditSink,
	previews PreviewGetter, consumer domain.PreviewConsumer, reader domain.AdjSnapshotReader, cfg AdjExecuteConfig,
) *AdjExecuteStep {
	return &AdjExecuteStep{
		runner: runner, writer: writer, calls: calls, audit: audit, previews: previews, consumer: consumer,
		reader: reader, cfg: cfg, caller: newW2Caller(writer, calls, cfg.CallTimeout, cfg.BusyRetries), now: time.Now,
	}
}

// WithClock overrides the clock (tests).
func (s *AdjExecuteStep) WithClock(now func() time.Time) *AdjExecuteStep {
	s.now = now
	return s
}

// WithSleep overrides the busy backoff wait (tests).
func (s *AdjExecuteStep) WithSleep(sleep func(ctx context.Context, d time.Duration) error) *AdjExecuteStep {
	s.caller.sleep = sleep
	return s
}

func adjSpecFor(op domain.AdjOperation) (adjOpSpec, error) {
	switch op {
	case domain.AdjOpValuate:
		return valuateSpec(), nil
	case domain.AdjOpApprove:
		return approveSpec(), nil
	case domain.AdjOpRestore:
		return restoreSpec(), nil
	default:
		return adjOpSpec{}, fmt.Errorf("%w: %q", ErrAdjOperationUnknown, op)
	}
}

// checkGates is G3, G1, G2 and the wiring: no I/O before they pass.
func (s *AdjExecuteStep) checkGates(cmd AdjExecuteCommand) error {
	denied := ErrAdjPermissionDenied
	if cmd.Operation == domain.AdjOpValuate {
		denied = ErrValuatePermissionDenied
	}
	if err := checkPermission(cmd.HasPermission, denied); err != nil {
		return err
	}
	if strings.TrimSpace(cmd.Actor) == "" {
		return domain.ErrActorRequired
	}
	if err := checkFlag(s.cfg.ValuationEnabled, flagValuationEnabled); err != nil {
		return err
	}
	if cmd.Operation == domain.AdjOpApprove {
		if err := checkFlag(s.cfg.AdjApproveEnabled, flagAdjApproveEnabled); err != nil {
			return err
		}
	}
	if err := checkWriter(s.writer); err != nil {
		return err
	}
	switch {
	case s.calls == nil:
		return ErrCallLogNotConfigured
	case s.reader == nil:
		return ErrAdjSnapshotReaderNotConfigured
	case s.previews == nil || s.consumer == nil:
		return ErrPreviewNotConfigured
	}
	return nil
}

// checkPreview is G4 and G5 against the stored preview.
func (s *AdjExecuteStep) checkPreview(ctx context.Context, cmd AdjExecuteCommand) (domain.ValuationPreview, error) {
	if strings.TrimSpace(cmd.PreviewID) == "" {
		return domain.ValuationPreview{}, fmt.Errorf("%w: preview_id is required", domain.ErrPreviewRequired)
	}
	p, err := s.previews.Get(ctx, cmd.PreviewID)
	if errors.Is(err, domain.ErrPreviewNotFound) {
		return p, fmt.Errorf("%w: %s", domain.ErrPreviewRequired, cmd.PreviewID)
	}
	if err != nil {
		return p, fmt.Errorf("read preview: %w", err)
	}
	switch {
	case p.Status == domain.PreviewConsumed:
		return p, domain.ErrPreviewConsumed
	case p.Status == domain.PreviewExpired, p.Status == domain.PreviewOpen && !s.now().Before(p.ExpiresAt):
		return p, domain.ErrPreviewExpired
	case p.Status != domain.PreviewOpen:
		return p, fmt.Errorf("%w: preview is %s", domain.ErrPreviewStale, p.Status)
	case p.BatchID != cmd.BatchID || p.Operation != cmd.Operation:
		return p, fmt.Errorf("%w: preview is for batch %d %s", domain.ErrPreviewRequired, p.BatchID, p.Operation)
	case cmd.ConfirmSetHash != p.SetHash || cmd.ConfirmText != p.ConfirmText:
		return p, fmt.Errorf("%w: set hash or confirmation text differs from the preview", domain.ErrConfirmMismatch)
	}
	return p, nil
}

// Run executes the operation. A failed / busy / unknown call keeps the batch
// status (its error text and summary commit) and is returned so the job
// fails.
func (s *AdjExecuteStep) Run(ctx context.Context, cmd AdjExecuteCommand, progress ProgressFunc) (AdjExecuteSummary, error) {
	spec, err := adjSpecFor(cmd.Operation)
	if err != nil {
		return AdjExecuteSummary{}, err
	}
	if err := s.checkGates(cmd); err != nil {
		return AdjExecuteSummary{}, err
	}
	p, err := s.checkPreview(ctx, cmd)
	if err != nil {
		return AdjExecuteSummary{}, err
	}
	var (
		sum     AdjExecuteSummary
		callErr error
	)
	err = s.runner.RunLocked(ctx, cmd.BatchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.AdjExecStore)
		if !ok {
			return ErrAdjStoreUnsupported
		}
		var err error
		sum, callErr, err = s.execLocked(ctx, st, spec, cmd, p, progress)
		return err
	})
	if err != nil {
		return AdjExecuteSummary{}, err
	}
	s.emitAudit(ctx, spec.auditOp, sum, cmd.Actor)
	if callErr != nil {
		return sum, fmt.Errorf("%s batch %d: %w", spec.callKey, cmd.BatchID, callErr)
	}
	return sum, nil
}

// execLocked runs inside the G11 transaction. callErr is a call outcome
// recorded on the (committed) batch; err aborts the transaction.
func (s *AdjExecuteStep) execLocked(ctx context.Context, st domain.AdjExecStore, spec adjOpSpec, cmd AdjExecuteCommand,
	p domain.ValuationPreview, progress ProgressFunc,
) (AdjExecuteSummary, error, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return AdjExecuteSummary{}, nil, err
	}
	if err := checkAdjBatch(ctx, st, spec, b); err != nil {
		return AdjExecuteSummary{}, nil, err
	}
	if err := s.consumer.Consume(ctx, p.ID, strings.TrimSpace(cmd.Actor), s.now()); err != nil {
		return AdjExecuteSummary{}, nil, fmt.Errorf("consume preview: %w", err)
	}
	progress.report(ctx, progressRead)
	sum := s.newSummary(b, spec, p)
	set, done, err := s.reprobe(ctx, st, spec, b, p)
	if err != nil {
		return AdjExecuteSummary{}, nil, err
	}
	sum.Heads, sum.Items = set.heads, len(set.rows)
	prev := b.Status()
	if done {
		// G6 / G9: the effect is already visible: finalize PG only.
		sum.AlreadyDone = true
		return sum, nil, s.finish(ctx, st, spec, b, prev, cmd.Actor, &sum)
	}
	out, err := s.caller.run(ctx, w2Call{
		key: spec.callKey, batchID: b.ID(), jobID: cmd.JobID, actor: cmd.Actor, fn: spec.call(b.ID(), cmd.Actor),
		params: map[string]any{"batch_id": b.ID(), "period": b.Period(), "preview_id": p.ID, "writer_mode": string(s.writer.Mode)},
	})
	if err != nil {
		return AdjExecuteSummary{}, nil, err
	}
	progress.report(ctx, progressCompute)
	recordOutcome(&sum, out)
	callErr := s.resolveOutcome(ctx, spec, b, set, out, &sum)
	progress.report(ctx, progressPersist)
	if callErr != nil {
		return sum, callErr, s.hold(ctx, st, spec, b, prev, callErr, &sum)
	}
	return sum, nil, s.finish(ctx, st, spec, b, prev, cmd.Actor, &sum)
}

// checkAdjBatch checks the batch (LIVE, status, not needs-repush) and G10.
func checkAdjBatch(ctx context.Context, st domain.AdjExecStore, spec adjOpSpec, b *domain.Batch) error {
	if b.Mode() == domain.ModeShadow {
		return fmt.Errorf("%w: batch %d", domain.ErrShadowNotPushable, b.ID())
	}
	if b.NeedsRepush() {
		return domain.ErrNeedsRepush
	}
	if err := spec.checkBatch(ctx, st, b); err != nil {
		return err
	}
	return checkPeriodLocked(ctx, st, b.Period())
}

// reprobe is G6 and G7a: re-read the period read-only, refuse posted heads,
// rebuild the set, compare its hash with the preview and run V-07. done is
// true when the hash differs because the effect is already visible.
func (s *AdjExecuteStep) reprobe(ctx context.Context, st domain.AdjExecStore, spec adjOpSpec, b *domain.Batch,
	p domain.ValuationPreview,
) (adjSet, bool, error) {
	rows, err := s.reader.SnapshotAdjRows(ctx, b.Period())
	if err != nil {
		return adjSet{}, false, fmt.Errorf("re-probe ADJ rows: %w", err)
	}
	eligible, excl := domain.SplitAdjSnapshot(rows)
	if excl.PostedHeads > 0 {
		return adjSet{}, false, fmt.Errorf("%w: %d posted head(s) in %s", domain.ErrPeriodPosted, excl.PostedHeads, b.Period())
	}
	if spec.op != domain.AdjOpApprove && excl.ApprovedHeads > p.Excluded.ApprovedHeads {
		return adjSet{}, false, fmt.Errorf("%w: %d head(s) approved since the preview", ErrAdjIneligible,
			excl.ApprovedHeads-p.Excluded.ApprovedHeads)
	}
	stdRows, err := st.ListStdRows(ctx)
	if err != nil {
		return adjSet{}, false, fmt.Errorf("re-probe std rows: %w", err)
	}
	set, err := spec.selectSet(rows, eligible, stdRows, b.ID())
	if err != nil {
		return adjSet{}, false, err
	}
	if domain.AdjSetHash(set.rows) != p.SetHash {
		if spec.alreadyDone(rows, eligible, stdRows, b.ID()) {
			return set, true, nil
		}
		return adjSet{}, false, fmt.Errorf("%w: the ADJ set changed since the preview", domain.ErrPreviewStale)
	}
	if spec.validate {
		if err := checkV07(b.Period(), set.rows, stdRows); err != nil {
			return adjSet{}, false, err
		}
	}
	return set, false, nil
}

// checkV07 is G7a over the whole set: one offending head refuses all.
func checkV07(period string, rows []domain.AdjSnapshotRow, stdRows []domain.StdRow) error {
	v07 := validation.PrevalidatePeriodSet(period, validation.ProjectRates(adjSetHeads(rows), stdRows))
	if !v07.OK() {
		return fmt.Errorf("%w: V-07 period-set pre-validation refused %d head(s): %v",
			domain.ErrValidationFailed, len(v07.Offending), v07.OffendingHeadIDs())
	}
	return nil
}

func recordOutcome(sum *AdjExecuteSummary, out w2Outcome) {
	sum.CallID, sum.CallStatus, sum.Attempts = out.callID, string(out.fin.Status), out.attempts
	sum.OraCode, sum.Error = out.fin.OraCode, out.fin.Error
	sum.OracleSummary = truncateText(out.summary.Text)
	sum.AlreadyDone = out.alreadyDone()
}

// resolveOutcome turns the call outcome into nil (success) or the error the
// batch is held with. An UNKNOWN outcome is resolved by a read-only probe.
func (s *AdjExecuteStep) resolveOutcome(ctx context.Context, spec adjOpSpec, b *domain.Batch, set adjSet,
	out w2Outcome, sum *AdjExecuteSummary,
) error {
	switch out.fin.Status {
	case domain.OracleCallSuccess:
		return nil
	case domain.OracleCallUnknown:
		return s.probeUnknown(ctx, spec, b, set, out, sum)
	default:
		return fmt.Errorf("%w: %s %s: %w", ErrAdjCallFailed, out.fin.Status, out.fin.OraCode, out.writeErr)
	}
}

// probeUnknown re-reads the period and checks the effect on every expected
// item: all → success, none → failed, part → UNKNOWN_PARTIAL (alert, hold).
func (s *AdjExecuteStep) probeUnknown(ctx context.Context, spec adjOpSpec, b *domain.Batch, set adjSet,
	out w2Outcome, sum *AdjExecuteSummary,
) error {
	rows, err := s.reader.SnapshotAdjRows(context.WithoutCancel(ctx), b.Period())
	if err != nil {
		res := UnknownResolution{Outcome: UnknownPartial, Total: len(set.expected)}
		sum.Resolution = &res
		log.Error().Err(err).Int64("batch_id", b.ID()).Str("key", spec.callKey).
			Msg("ALERT erp w2: UNKNOWN outcome and the re-probe failed; batch held")
		return fmt.Errorf("%w: re-probe failed: %w", ErrAdjUnknownPartial, err)
	}
	res := ResolveUnknown(rows, set.expected, spec.done(b.ID()))
	sum.Resolution = &res
	switch res.Outcome {
	case UnknownResolvedOK:
		return nil
	case UnknownResolvedFailed:
		return fmt.Errorf("%w: UNKNOWN resolved to FAILED (0/%d): %w", ErrAdjCallFailed, res.Total, out.writeErr)
	default:
		log.Error().Int64("batch_id", b.ID()).Str("key", spec.callKey).Int("done", res.Done).Int("total", res.Total).
			Msg("ALERT erp w2: UNKNOWN_PARTIAL outcome; batch held for manual resolution")
		return fmt.Errorf("%w: %d/%d item(s) show the effect", ErrAdjUnknownPartial, res.Done, res.Total)
	}
}

// finish records the success (one PG tx with the operation's changes).
func (s *AdjExecuteStep) finish(ctx context.Context, st domain.AdjExecStore, spec adjOpSpec, b *domain.Batch,
	prev domain.BatchStatus, actor string, sum *AdjExecuteSummary,
) error {
	if err := spec.finalize(s, ctx, st, b, actor, sum); err != nil {
		return err
	}
	if b.Status() != domain.StatusFailed {
		// RESTORE ends FAILED with its own reason; keep it.
		b.SetErrorText("")
	}
	sum.Status = string(b.Status())
	if err := setStepSummary(b, spec.summaryKey, *sum); err != nil {
		return err
	}
	return st.Save(ctx, b, prev, domain.Invalidation{})
}

// hold keeps the batch status and records the error text and summary.
func (s *AdjExecuteStep) hold(ctx context.Context, st domain.AdjExecStore, spec adjOpSpec, b *domain.Batch,
	prev domain.BatchStatus, callErr error, sum *AdjExecuteSummary,
) error {
	b.SetErrorText(truncateText(fmt.Sprintf("%s: %s", spec.callKey, callErr.Error())))
	sum.Status = string(b.Status())
	if err := setStepSummary(b, spec.summaryKey, *sum); err != nil {
		return err
	}
	return st.Save(ctx, b, prev, domain.Invalidation{})
}

func (s *AdjExecuteStep) newSummary(b *domain.Batch, spec adjOpSpec, p domain.ValuationPreview) AdjExecuteSummary {
	return AdjExecuteSummary{
		BatchID: b.ID(), Period: b.Period(), Operation: string(spec.op), PreviewID: p.ID,
		WriterMode: string(s.writer.Mode), RunAt: s.now(),
	}
}

// emitAudit records the operation (best effort: the outcome is committed
// and the call log holds the Oracle-side record).
func (s *AdjExecuteStep) emitAudit(ctx context.Context, op string, sum AdjExecuteSummary, actor string) {
	emitBatchAudit(ctx, s.audit, op, sum.BatchID, sum, actor)
}

// emitBatchAudit is the shared best-effort batch audit emit.
func emitBatchAudit(ctx context.Context, audit AuditSink, op string, batchID int64, v any, actor string) {
	if audit == nil {
		return
	}
	after, err := json.Marshal(v)
	if err != nil {
		log.Warn().Err(err).Int64("batch_id", batchID).Msg("erp adj: marshal audit")
		return
	}
	if err := audit.Emit(ctx, auditdomain.NewInput{
		EntityType: auditEntityErpBatch, EntityID: batchID, Operation: op,
		AfterData: string(after), UserID: strings.TrimSpace(actor),
	}); err != nil {
		log.Warn().Err(err).Int64("batch_id", batchID).Str("op", op).Msg("erp adj: audit emit failed")
	}
}

// --- VALUATE ---------------------------------------------------------------

// valuateSpec: PUSHED only; the set is every eligible row with its
// projection; the effect is ADJI_FLEX_13 = batch id on every projected row;
// success supersedes the prior active batch and moves the batch to VALUATED
// in one PG transaction.
func valuateSpec() adjOpSpec {
	return adjOpSpec{
		op: domain.AdjOpValuate, callKey: domain.CallKeyW2ValuateAdj, summaryKey: "adj_valuate",
		auditOp: auditdomain.OpErpValuate, validate: true,
		checkBatch: func(_ context.Context, _ domain.AdjExecStore, b *domain.Batch) error {
			return requireStatus(b, "VALUATE_ADJ", domain.StatusPushed)
		},
		selectSet: func(_, eligible []domain.AdjSnapshotRow, std []domain.StdRow, batchID int64) (adjSet, error) {
			projected, _, err := projectSnapshot(eligible, std, batchID)
			if err != nil {
				return adjSet{}, err
			}
			return adjSet{rows: projected, expected: projectedItems(projected), heads: len(adjSetHeads(projected))}, nil
		},
		done: func(batchID int64) rowDoneFunc {
			return func(r domain.AdjSnapshotRow) bool { return stampedBy(r, batchID) }
		},
		alreadyDone: func(_, eligible []domain.AdjSnapshotRow, std []domain.StdRow, batchID int64) bool {
			projected, _, err := projectSnapshot(eligible, std, batchID)
			if err != nil {
				return false
			}
			return ResolveUnknown(eligible, projectedItems(projected), func(r domain.AdjSnapshotRow) bool {
				return stampedBy(r, batchID)
			}).Outcome == UnknownResolvedOK
		},
		call: func(batchID int64, _ string) func(context.Context, domain.OracleWriter) (domain.Summary, error) {
			return func(ctx context.Context, w domain.OracleWriter) (domain.Summary, error) {
				return w.ValuateAdj(ctx, batchID)
			}
		},
		finalize: (*AdjExecuteStep).finalizeValuate,
	}
}

// finalizeValuate: prior active → SUPERSEDED, then batch → VALUATED.
func (s *AdjExecuteStep) finalizeValuate(ctx context.Context, st domain.AdjExecStore, b *domain.Batch, actor string,
	sum *AdjExecuteSummary,
) error {
	at := s.now()
	prior, err := st.ActiveForUpdate(ctx, b.Period())
	switch {
	case errors.Is(err, domain.ErrBatchNotFound):
	case err != nil:
		return fmt.Errorf("load prior active batch: %w", err)
	default:
		priorStatus := prior.Status()
		if _, err := prior.Transition(domain.StatusSuperseded, actor, at); err != nil {
			return fmt.Errorf("supersede batch %d: %w", prior.ID(), err)
		}
		if err := st.SaveOther(ctx, prior, priorStatus); err != nil {
			return fmt.Errorf("supersede batch %d: %w", prior.ID(), err)
		}
		sum.SupersededBatch = prior.ID()
	}
	_, err = b.Transition(domain.StatusValuated, actor, at)
	return err
}

// requireStatus refuses a batch outside the allowed statuses.
func requireStatus(b *domain.Batch, what string, allowed ...domain.BatchStatus) error {
	for _, a := range allowed {
		if b.Status() == a {
			return nil
		}
	}
	return fmt.Errorf("%w: %s in %s", ErrStepNotAllowed, what, b.Status())
}
