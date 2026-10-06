package erpintegration

// ack_warnings_handler.go implements the V-05 / V-08w warnings
// acknowledgement (plan-06 P5-T2 step 2; design Part 1 §5.3, Part 2 §7). The
// ack is bound to the warning-set hash of the last validate run and stored
// with the rule diff against the baseline in ceib_summary.ack; the audit
// event is ERP_WARN_ACK. The RBAC check itself is in delivery (P6-T3): the
// command carries its outcome and a false one is refused here too.

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
)

// auditEntityErpBatch is the audit entity of batch-level events.
const auditEntityErpBatch = "cst_erp_int_batch"

// Ack errors.
var (
	// ErrAckPermissionDenied is returned when the caller lacks the ack
	// permission.
	ErrAckPermissionDenied = errors.New("erpintegration: permission denied to acknowledge warnings")
	// ErrAckHashMismatch is returned when the acked warning set is not the
	// one of the last validate run (re-validated or re-derived since).
	ErrAckHashMismatch = errors.New("erpintegration: warning set changed; re-validate and review again")
	// ErrNothingToAck is returned when the last validate run has no warnings
	// (or the batch was never validated for its current derivation).
	ErrNothingToAck = errors.New("erpintegration: no validated warnings to acknowledge")
)

// AckWarningsCommand acknowledges the warnings of a batch. WarningSetHash is
// the hash the reviewer saw (ceib_summary.validate.warning_set_hash).
// HasPermission is the delivery-layer RBAC outcome.
type AckWarningsCommand struct {
	BatchID        int64
	Actor          string
	WarningSetHash string
	HasPermission  bool
}

// AckRecord is ceib_summary.ack: who acked which warning set, and the rule
// diff against the baseline that was reviewed.
type AckRecord struct {
	WarningSetHash  string    `json:"warning_set_hash"`
	By              string    `json:"by"`
	At              time.Time `json:"at"`
	Warnings        int       `json:"warnings"`
	BaselineBatchID *int64    `json:"baseline_batch_id,omitempty"`
	BaselinePeriod  string    `json:"baseline_period,omitempty"`
	RuleDiff        []string  `json:"rule_diff"`
}

// AckWarningsResult is the outcome of an ack.
type AckWarningsResult struct {
	Ack       AckRecord `json:"ack"`
	Validated bool      `json:"validated"`
	Status    string    `json:"batch_status"`
}

// AckWarningsHandler records the warnings ack.
type AckWarningsHandler struct {
	runner domain.BatchTxRunner
	audit  AuditSink
	now    func() time.Time
}

// NewAckWarningsHandler builds the handler; audit may be nil.
func NewAckWarningsHandler(runner domain.BatchTxRunner, audit AuditSink) *AckWarningsHandler {
	return &AckWarningsHandler{runner: runner, audit: audit, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *AckWarningsHandler) WithClock(now func() time.Time) *AckWarningsHandler {
	h.now = now
	return h
}

// Handle acknowledges the warnings under the G11 lock. The batch must be
// DERIVED or VALIDATED, not pushed, and validated for its current
// derivation with exactly cmd.WarningSetHash. When that validate run had 0
// errors a DERIVED batch moves to VALIDATED (the ack was the only blocker).
func (h *AckWarningsHandler) Handle(ctx context.Context, cmd AckWarningsCommand) (AckWarningsResult, error) {
	if !cmd.HasPermission {
		return AckWarningsResult{}, ErrAckPermissionDenied
	}
	hash := strings.TrimSpace(cmd.WarningSetHash)
	if hash == "" {
		return AckWarningsResult{}, fmt.Errorf("%w: warning set hash is required", ErrAckHashMismatch)
	}
	var res AckWarningsResult
	err := h.runner.RunLocked(ctx, cmd.BatchID, func(ctx context.Context, st domain.BatchStore) error {
		var err error
		res, err = h.ackLocked(ctx, st, cmd.Actor, hash)
		return err
	})
	if err != nil {
		return AckWarningsResult{}, err
	}
	h.emitAudit(ctx, cmd.BatchID, res)
	return res, nil
}

func (h *AckWarningsHandler) ackLocked(ctx context.Context, st domain.BatchStore, actor, hash string) (AckWarningsResult, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return AckWarningsResult{}, err
	}
	prev := b.Status()
	if err := checkValidateStatus(b); err != nil {
		return AckWarningsResult{}, err
	}
	last, err := currentValidateSummary(b)
	if err != nil {
		return AckWarningsResult{}, err
	}
	if last.WarningSetHash != hash {
		return AckWarningsResult{}, ErrAckHashMismatch
	}
	at := h.now()
	if err := b.AckWarnings(actor, at); err != nil {
		return AckWarningsResult{}, err
	}
	stamp := b.WarningsAck()
	rec := AckRecord{
		WarningSetHash: hash, By: stamp.By, At: stamp.At, Warnings: last.Warnings,
		BaselineBatchID: last.DryRun.BaselineBatchID, BaselinePeriod: last.DryRun.BaselinePeriod,
		RuleDiff: append([]string{}, last.DryRun.RuleDiff...),
	}
	if err := setStepSummary(b, summaryKeyAck, rec); err != nil {
		return AckWarningsResult{}, err
	}
	if last.Errors == 0 && b.Status() == domain.StatusDerived {
		if _, err := b.Transition(domain.StatusValidated, actor, at); err != nil {
			return AckWarningsResult{}, err
		}
	}
	if err := st.Save(ctx, b, prev, domain.Invalidation{}); err != nil {
		return AckWarningsResult{}, err
	}
	return AckWarningsResult{Ack: rec, Validated: b.Status() == domain.StatusValidated, Status: string(b.Status())}, nil
}

// currentValidateSummary returns ceib_summary.validate when it belongs to
// the current derivation (same rule hash and derive run) and has warnings.
func currentValidateSummary(b *domain.Batch) (ValidateSummary, error) {
	var last ValidateSummary
	if !readSummaryEntry(b.Summary(), StepValidate, &last) || last.WarningSetHash == "" {
		return ValidateSummary{}, ErrNothingToAck
	}
	if last.RuleHash != b.RuleHash() || !sameTime(last.DeriveRunAt, summaryDeriveRunAt(b.Summary())) {
		return ValidateSummary{}, fmt.Errorf("%w: batch re-derived since the last validate", ErrAckHashMismatch)
	}
	return last, nil
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// emitAudit records ERP_WARN_ACK (best effort: the ack is committed).
func (h *AckWarningsHandler) emitAudit(ctx context.Context, batchID int64, res AckWarningsResult) {
	if h.audit == nil {
		return
	}
	after, err := json.Marshal(res)
	if err != nil {
		log.Warn().Err(err).Int64("batch_id", batchID).Msg("erp warn ack: marshal audit")
		return
	}
	if err := h.audit.Emit(ctx, auditdomain.NewInput{
		EntityType: auditEntityErpBatch,
		EntityID:   batchID,
		Operation:  auditdomain.OpErpWarnAck,
		AfterData:  string(after),
		UserID:     res.Ack.By,
	}); err != nil {
		log.Warn().Err(err).Int64("batch_id", batchID).Msg("erp warn ack: audit emit failed")
	}
}

// readAckRecord returns ceib_summary.ack, if any.
func readAckRecord(summary []byte) (AckRecord, bool) {
	var rec AckRecord
	if !readSummaryEntry(summary, summaryKeyAck, &rec) {
		return AckRecord{}, false
	}
	return rec, true
}
