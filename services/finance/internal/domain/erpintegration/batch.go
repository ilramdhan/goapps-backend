package erpintegration

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// maxActorLen mirrors the VARCHAR(64) *_by columns of cst_erp_int_batch.
const maxActorLen = 64

var (
	batchPeriodRe = regexp.MustCompile(`^[0-9]{4}(0[1-9]|1[0-2])$`)
	ruleHashRe    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidateBatchPeriod checks the YYYYMM format of ceib_period.
func ValidateBatchPeriod(period string) error {
	if !batchPeriodRe.MatchString(period) {
		return fmt.Errorf("%w: %q", ErrInvalidBatchPeriod, period)
	}
	return nil
}

func normalizeActor(actor string) (string, error) {
	a := strings.TrimSpace(actor)
	if a == "" || len(a) > maxActorLen {
		return "", ErrActorRequired
	}
	return a, nil
}

// ControlTotals are the batch control totals computed from OK std rows and
// pushed to GSB_SUM_* (design §5.2). Sums are held at 5 dp (NUMERIC(24,5)).
type ControlTotals struct {
	rowCount int64
	sumStd   decimal.Decimal
	sumConv  decimal.Decimal
	sumPvl   decimal.Decimal
}

// NewControlTotals validates and rounds (half away from zero, 5 dp) the
// totals. A negative row count is rejected.
func NewControlTotals(rowCount int64, sumStd, sumConv, sumPvl decimal.Decimal) (ControlTotals, error) {
	if rowCount < 0 {
		return ControlTotals{}, fmt.Errorf("%w: row count %d", ErrInvalidControlTotals, rowCount)
	}
	return ControlTotals{
		rowCount: rowCount,
		sumStd:   Round5(sumStd),
		sumConv:  Round5(sumConv),
		sumPvl:   Round5(sumPvl),
	}, nil
}

// RowCount is the number of OK rows.
func (c ControlTotals) RowCount() int64 { return c.rowCount }

// SumStd is Σ std over OK rows.
func (c ControlTotals) SumStd() decimal.Decimal { return c.sumStd }

// SumConv is Σ conv over OK rows.
func (c ControlTotals) SumConv() decimal.Decimal { return c.sumConv }

// SumPvl is Σ pvl over OK rows.
func (c ControlTotals) SumPvl() decimal.Decimal { return c.sumPvl }

// Equal reports value equality (decimal-aware).
func (c ControlTotals) Equal(o ControlTotals) bool {
	return c.rowCount == o.rowCount && c.sumStd.Equal(o.sumStd) &&
		c.sumConv.Equal(o.sumConv) && c.sumPvl.Equal(o.sumPvl)
}

// ActorStamp is a (time, actor) audit pair.
type ActorStamp struct {
	At time.Time
	By string
}

// Batch is the aggregate root of one ERP integration run for a period
// (cst_erp_int_batch; design Part 1 §4.2, §5.2, §5.3).
//
// Invariants:
//   - status changes only through Transition, which enforces the §5.3 table;
//   - a SHADOW batch never leaves the shadow-allowed statuses (never PUSHED);
//   - once pushed, control totals, rule snapshot and rule hash are immutable;
//   - LOCKED, FAILED and SUPERSEDED are terminal.
type Batch struct {
	id     int64
	period string
	seq    int
	mode   BatchMode
	status BatchStatus

	jobID          string
	demandLoadedAt *time.Time

	ruleSnapshot []byte
	ruleHash     string
	totals       *ControlTotals

	summary []byte

	warningsAck *ActorStamp
	pushed      *ActorStamp
	valuated    *ActorStamp
	reconciled  *time.Time
	adjApproved *ActorStamp
	locked      *ActorStamp

	needsRepush bool
	errText     string

	createdAt time.Time
	createdBy string
	updatedAt time.Time
	updatedBy string
}

// NewBatch creates a DRAFT batch. The id is assigned by the repository
// (BIGSERIAL); seq is 1..n per period and is also allocated by the repository,
// so it may be passed as 0 here and set via Reconstitute on reload.
func NewBatch(period string, mode BatchMode, actor string, now time.Time) (*Batch, error) {
	if err := ValidateBatchPeriod(period); err != nil {
		return nil, err
	}
	if _, err := ParseBatchMode(string(mode)); err != nil {
		return nil, err
	}
	a, err := normalizeActor(actor)
	if err != nil {
		return nil, err
	}
	return &Batch{
		period:    period,
		mode:      mode,
		status:    StatusDraft,
		createdAt: now,
		createdBy: a,
		updatedAt: now,
		updatedBy: a,
	}, nil
}

// BatchState is the persisted form used to rebuild a Batch from storage.
type BatchState struct {
	ID             int64
	Period         string
	Seq            int
	Mode           BatchMode
	Status         BatchStatus
	JobID          string
	DemandLoadedAt *time.Time
	RuleSnapshot   []byte
	RuleHash       string
	Totals         *ControlTotals
	Summary        []byte
	WarningsAck    *ActorStamp
	Pushed         *ActorStamp
	Valuated       *ActorStamp
	ReconciledAt   *time.Time
	AdjApproved    *ActorStamp
	Locked         *ActorStamp
	NeedsRepush    bool
	Error          string
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      time.Time
	UpdatedBy      string
}

// ReconstituteBatch rebuilds a Batch from storage. It validates the enum and
// format fields so a corrupted row is rejected instead of silently loaded.
func ReconstituteBatch(s BatchState) (*Batch, error) {
	if err := ValidateBatchPeriod(s.Period); err != nil {
		return nil, err
	}
	if s.Seq < 1 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidBatchSeq, s.Seq)
	}
	mode, err := ParseBatchMode(string(s.Mode))
	if err != nil {
		return nil, err
	}
	st, err := ParseBatchStatus(string(s.Status))
	if err != nil {
		return nil, err
	}
	if mode == ModeShadow && !st.AllowedInShadow() {
		return nil, fmt.Errorf("%w: status %s", ErrShadowNotPushable, st)
	}
	if s.RuleHash != "" && !ruleHashRe.MatchString(s.RuleHash) {
		return nil, ErrInvalidRuleHash
	}
	return &Batch{
		id: s.ID, period: s.Period, seq: s.Seq, mode: mode, status: st,
		jobID: s.JobID, demandLoadedAt: copyTime(s.DemandLoadedAt),
		ruleSnapshot: copyBytes(s.RuleSnapshot), ruleHash: s.RuleHash, totals: copyTotals(s.Totals),
		summary:     copyBytes(s.Summary),
		warningsAck: copyStamp(s.WarningsAck), pushed: copyStamp(s.Pushed), valuated: copyStamp(s.Valuated),
		reconciled: copyTime(s.ReconciledAt), adjApproved: copyStamp(s.AdjApproved), locked: copyStamp(s.Locked),
		needsRepush: s.NeedsRepush, errText: s.Error,
		createdAt: s.CreatedAt, createdBy: s.CreatedBy, updatedAt: s.UpdatedAt, updatedBy: s.UpdatedBy,
	}, nil
}

// State returns the persisted form of the batch (deep copy).
func (b *Batch) State() BatchState {
	return BatchState{
		ID: b.id, Period: b.period, Seq: b.seq, Mode: b.mode, Status: b.status,
		JobID: b.jobID, DemandLoadedAt: copyTime(b.demandLoadedAt),
		RuleSnapshot: copyBytes(b.ruleSnapshot), RuleHash: b.ruleHash, Totals: copyTotals(b.totals),
		Summary:     copyBytes(b.summary),
		WarningsAck: copyStamp(b.warningsAck), Pushed: copyStamp(b.pushed), Valuated: copyStamp(b.valuated),
		ReconciledAt: copyTime(b.reconciled), AdjApproved: copyStamp(b.adjApproved), Locked: copyStamp(b.locked),
		NeedsRepush: b.needsRepush, Error: b.errText,
		CreatedAt: b.createdAt, CreatedBy: b.createdBy, UpdatedAt: b.updatedAt, UpdatedBy: b.updatedBy,
	}
}

// Getters.

// ID is ceib_batch_id (= GSB_BATCH_ID = FLEX_13); 0 until persisted.
func (b *Batch) ID() int64 { return b.id }

// Period is YYYYMM.
func (b *Batch) Period() string { return b.period }

// Seq is the 1..n sequence per period (0 until persisted).
func (b *Batch) Seq() int { return b.seq }

// Mode is LIVE or SHADOW.
func (b *Batch) Mode() BatchMode { return b.mode }

// Status is the current lifecycle status.
func (b *Batch) Status() BatchStatus { return b.status }

// JobID is the last job_execution id.
func (b *Batch) JobID() string { return b.jobID }

// DemandLoadedAt is the V-10 freshness anchor.
func (b *Batch) DemandLoadedAt() *time.Time { return copyTime(b.demandLoadedAt) }

// RuleSnapshot is the canonical rule set JSON stored at derive.
func (b *Batch) RuleSnapshot() []byte { return copyBytes(b.ruleSnapshot) }

// RuleHash is the SHA-256 hex of the rule snapshot ("" if not derived).
func (b *Batch) RuleHash() string { return b.ruleHash }

// Totals are the control totals (nil if not derived).
func (b *Batch) Totals() *ControlTotals { return copyTotals(b.totals) }

// Summary is the step summary JSON.
func (b *Batch) Summary() []byte { return copyBytes(b.summary) }

// WarningsAck is the V-05/V-08w acknowledgement.
func (b *Batch) WarningsAck() *ActorStamp { return copyStamp(b.warningsAck) }

// Pushed is the push stamp.
func (b *Batch) Pushed() *ActorStamp { return copyStamp(b.pushed) }

// Valuated is the VALUATE_ADJ stamp.
func (b *Batch) Valuated() *ActorStamp { return copyStamp(b.valuated) }

// ReconciledAt is the recon time.
func (b *Batch) ReconciledAt() *time.Time { return copyTime(b.reconciled) }

// AdjApproved is the ADJ approval stamp (D-D2: status stays RECONCILED).
func (b *Batch) AdjApproved() *ActorStamp { return copyStamp(b.adjApproved) }

// Locked is the LOCK_BATCH stamp.
func (b *Batch) Locked() *ActorStamp { return copyStamp(b.locked) }

// NeedsRepush is set when the period is unlocked after push (§5.5).
func (b *Batch) NeedsRepush() bool { return b.needsRepush }

// ErrorText is the last fatal error text (ceib_error).
func (b *Batch) ErrorText() string { return b.errText }

// CreatedAt / CreatedBy / UpdatedAt / UpdatedBy are the row audit columns.
func (b *Batch) CreatedAt() time.Time { return b.createdAt }

// CreatedBy is the creating actor.
func (b *Batch) CreatedBy() string { return b.createdBy }

// UpdatedAt is the last change time.
func (b *Batch) UpdatedAt() time.Time { return b.updatedAt }

// UpdatedBy is the last changing actor.
func (b *Batch) UpdatedBy() string { return b.updatedBy }

// IsFrozen reports whether totals and rule hash are immutable (pushed).
func (b *Batch) IsFrozen() bool { return b.pushed != nil }

// Transition moves the batch to status `to`, enforcing the §5.3 table and the
// aggregate invariants, and applies the step's side effects:
//   - to DEMAND_LOADED: demand_loaded_at = at; downstream artifacts cleared;
//   - to COVERED / DERIVED: downstream artifacts cleared (see InvalidationFor);
//   - VALIDATED -> PUSHED: requires rule hash + totals; stamps pushed;
//   - PUSHED -> VALUATED: stamps valuated;
//   - to RECONCILED: stamps reconciled_at;
//   - RECONCILED -> VALUATED (re-recon DIFF): clears reconciled_at;
//   - to LOCKED: stamps locked.
//
// It returns the artifacts the caller must invalidate in the same PG
// transaction. External guards (period lock G10, posted probe, V-results,
// feature flags) are checked by the application layer before calling.
func (b *Batch) Transition(to BatchStatus, actor string, at time.Time) (Invalidation, error) {
	a, err := normalizeActor(actor)
	if err != nil {
		return Invalidation{}, err
	}
	if !to.IsValid() {
		return Invalidation{}, fmt.Errorf("%w: %q", ErrInvalidBatchStatus, to)
	}
	from := b.status
	if err := b.checkTransition(from, to); err != nil {
		return Invalidation{}, err
	}
	inv := InvalidationFor(from, to)
	b.applyTransition(from, to, inv, a, at)
	return inv, nil
}

// checkTransition enforces the §5.3 table and the aggregate invariants.
func (b *Batch) checkTransition(from, to BatchStatus) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	if b.mode == ModeShadow && !to.AllowedInShadow() {
		return fmt.Errorf("%w: %s -> %s", ErrShadowNotPushable, from, to)
	}
	if b.needsRepush && blockedByRepush(to) {
		return fmt.Errorf("%w: %s -> %s", ErrNeedsRepush, from, to)
	}
	if from == StatusValidated && to == StatusPushed && (b.ruleHash == "" || b.totals == nil) {
		return ErrControlTotalsMissing
	}
	return nil
}

// blockedByRepush reports the forward statuses a needs-repush batch may not
// reach (§5.5: a new batch is required before recon/approve).
func blockedByRepush(to BatchStatus) bool {
	return to == StatusValuated || to == StatusReconciled || to == StatusLocked
}

// applyTransition applies invalidation, the step stamps and the new status.
func (b *Batch) applyTransition(from, to BatchStatus, inv Invalidation, actor string, at time.Time) {
	if inv.RuleSnapshot {
		b.ruleSnapshot, b.ruleHash, b.totals = nil, "", nil
	}
	if inv.WarningsAck {
		b.warningsAck = nil
	}
	stamp := &ActorStamp{At: at, By: actor}
	switch {
	case to == StatusDemandLoaded:
		t := at
		b.demandLoadedAt = &t
	case from == StatusValidated && to == StatusPushed:
		b.pushed = stamp
	case from == StatusPushed && to == StatusValuated:
		b.valuated = stamp
	case to == StatusReconciled:
		t := at
		b.reconciled = &t
	case from == StatusReconciled && to == StatusValuated:
		b.reconciled = nil
	case to == StatusLocked:
		b.locked = stamp
	}
	b.status = to
	b.touch(actor, at)
}

// SetDerivation stores the rule snapshot, rule hash and control totals. It is
// allowed only while DERIVED or VALIDATED (derive, and the re-verification at
// validate) and never once the batch is pushed.
func (b *Batch) SetDerivation(snapshot []byte, ruleHash string, totals ControlTotals, actor string, at time.Time) error {
	a, err := normalizeActor(actor)
	if err != nil {
		return err
	}
	if b.IsFrozen() {
		return ErrBatchFrozen
	}
	if b.status != StatusDerived && b.status != StatusValidated {
		return fmt.Errorf("%w: set derivation in %s", ErrInvalidTransition, b.status)
	}
	if !ruleHashRe.MatchString(ruleHash) {
		return ErrInvalidRuleHash
	}
	b.ruleSnapshot = copyBytes(snapshot)
	b.ruleHash = ruleHash
	t := totals
	b.totals = &t
	b.touch(a, at)
	return nil
}

// AckWarnings records the V-05/V-08w acknowledgement (DERIVED or VALIDATED).
func (b *Batch) AckWarnings(actor string, at time.Time) error {
	a, err := normalizeActor(actor)
	if err != nil {
		return err
	}
	if b.status != StatusDerived && b.status != StatusValidated {
		return fmt.Errorf("%w: ack warnings in %s", ErrInvalidTransition, b.status)
	}
	b.warningsAck = &ActorStamp{At: at, By: a}
	b.touch(a, at)
	return nil
}

// MarkAdjApproved records the ADJ approval (step 13). It does not change the
// status and is allowed only while RECONCILED (D-D2).
func (b *Batch) MarkAdjApproved(actor string, at time.Time) error {
	a, err := normalizeActor(actor)
	if err != nil {
		return err
	}
	if b.status != StatusReconciled {
		return fmt.Errorf("%w: approve ADJ in %s", ErrInvalidTransition, b.status)
	}
	if b.needsRepush {
		return ErrNeedsRepush
	}
	b.adjApproved = &ActorStamp{At: at, By: a}
	b.touch(a, at)
	return nil
}

// MarkNeedsRepush flags the batch after a period unlock (§5.5). It applies to
// PUSHED, VALUATED and RECONCILED batches and reports whether it changed
// anything; other statuses are left untouched.
func (b *Batch) MarkNeedsRepush(actor string, at time.Time) (bool, error) {
	a, err := normalizeActor(actor)
	if err != nil {
		return false, err
	}
	switch b.status {
	case StatusPushed, StatusValuated, StatusReconciled:
	default:
		return false, nil
	}
	if b.needsRepush {
		return false, nil
	}
	b.needsRepush = true
	b.touch(a, at)
	return true, nil
}

// Fail moves the batch to FAILED with an error text (Abandon, fatal step
// error or RESTORE_ADJ). It is Transition(FAILED) plus the error message.
func (b *Batch) Fail(reason, actor string, at time.Time) error {
	if _, err := b.Transition(StatusFailed, actor, at); err != nil {
		return err
	}
	b.errText = strings.TrimSpace(reason)
	return nil
}

// SetJobID records the last job id.
func (b *Batch) SetJobID(jobID string) { b.jobID = jobID }

// SetSummary replaces the step summary JSON.
func (b *Batch) SetSummary(summary []byte) { b.summary = copyBytes(summary) }

// AllowedTransitions lists the statuses reachable from the current status,
// in lifecycle order, after the aggregate invariants (shadow, needs-repush,
// missing totals) are applied. It backs allowed_actions[] (P6).
func (b *Batch) AllowedTransitions() []BatchStatus {
	out := make([]BatchStatus, 0, len(allStatuses))
	for _, to := range allStatuses {
		if b.checkTransition(b.status, to) == nil {
			out = append(out, to)
		}
	}
	return out
}

func (b *Batch) touch(actor string, at time.Time) {
	b.updatedAt = at
	b.updatedBy = actor
}

func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

func copyStamp(s *ActorStamp) *ActorStamp {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

func copyTotals(t *ControlTotals) *ControlTotals {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
