package erpintegration

// validate_step.go implements the Validate step (plan-06 P5-T2; design Part 1
// §2 step 8, §5.3; Part 2 §7, §9.2). PG only, except the read-only V-10
// posted-head probe (ErpAdjHeadProber, SELECT only). It runs V-01..V-12 over
// the batch's std rows, stores the row findings in cesc_validation and the
// V-05 baseline in cesc_prev_std_cost, writes ceib_summary.validate (counts,
// warning-set hash, dry-run summary) and moves DERIVED -> VALIDATED only with
// 0 errors and every warning acknowledged for the same warning set.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration/validation"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// StepValidate is the erp_integration subtype and the ceib_summary key.
const StepValidate = "validate"

// summaryKeyAck is the ceib_summary key of the warnings acknowledgement
// record (AckRecord).
const summaryKeyAck = "ack"

// validateFindingSample bounds ValidateSummary.Findings and .Issues.
const validateFindingSample = 50

// ErrValidateStoreUnsupported is returned when the batch runner's store does
// not implement domain.ValidateStore.
var ErrValidateStoreUnsupported = errors.New("erpintegration: batch store does not support the validate step")

// ValidateSummary is the validate step summary (ceib_summary.validate and
// job result_summary).
type ValidateSummary struct {
	BatchID  int64          `json:"batch_id"`
	Period   string         `json:"period"`
	Rows     int64          `json:"rows"`
	Errors   int            `json:"errors"`
	Warnings int            `json:"warnings"`
	Infos    int            `json:"infos"`
	ByCode   map[string]int `json:"by_code"`
	// WarningSetHash binds an ack to the exact warning set (empty: none).
	WarningSetHash string `json:"warning_set_hash"`
	NeedsAck       bool   `json:"needs_ack"`
	Acked          bool   `json:"acked"`
	Validated      bool   `json:"validated"`
	Status         string `json:"batch_status"`
	RuleHash       string `json:"rule_hash"`
	// DeriveRunAt is ceib_summary.derive.run_at at validation time: an ack
	// is refused once a later derive replaced the rows.
	DeriveRunAt *time.Time `json:"derive_run_at,omitempty"`
	RowsUpdated int64      `json:"rows_updated"`
	ProbeError  string     `json:"probe_error,omitempty"`
	RunAt       time.Time  `json:"run_at"`
	// Findings are the batch / coverage / period-set findings (bounded).
	Findings []string `json:"findings,omitempty"`
	// Issues is a bounded sample of the row errors and warnings; the full
	// list lives on the std rows (cesc_validation).
	Issues []string      `json:"issues,omitempty"`
	DryRun DryRunSummary `json:"dry_run"`
}

// ValidateStep validates the std rows of a batch.
type ValidateStep struct {
	runner domain.BatchTxRunner
	prober domain.ErpAdjHeadProber
	maxAge time.Duration
	now    func() time.Time
}

// NewValidateStep builds the step. prober nil (no Oracle) leaves the V-10
// posted-head probe unrun, which is a V-10 error (fail closed). maxAge is
// erp_integration.demand_max_age (V-10; <= 0 uses the 24 h default).
func NewValidateStep(runner domain.BatchTxRunner, prober domain.ErpAdjHeadProber, maxAge time.Duration) *ValidateStep {
	return &ValidateStep{runner: runner, prober: prober, maxAge: maxAge, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *ValidateStep) WithClock(now func() time.Time) *ValidateStep {
	s.now = now
	return s
}

// Run validates under the G11 lock. Guards: status DERIVED or VALIDATED (a
// re-validation), not pushed, and for a LIVE batch the (P, ACTUAL) period
// lock (G10). Demand freshness is V-10 (a finding, not a guard), so a stale
// VALIDATED batch is revoked rather than left VALIDATED.
//
// A blocked outcome (errors, or warnings not acked for this warning set) is
// not a step failure: the job succeeds, the summary shows why and a
// VALIDATED batch is revoked to DERIVED.
func (s *ValidateStep) Run(ctx context.Context, batchID int64, actor string, progress ProgressFunc) (ValidateSummary, error) {
	var sum ValidateSummary
	err := s.runner.RunLocked(ctx, batchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.ValidateStore)
		if !ok {
			return ErrValidateStoreUnsupported
		}
		var err error
		sum, err = s.validateLocked(ctx, st, actor, progress)
		return err
	})
	if err != nil {
		return ValidateSummary{}, err
	}
	return sum, nil
}

// validateInputs is what the step read before running the validators.
type validateInputs struct {
	in         validation.Input
	baseline   *domain.ValidationBaseline
	probeError string
}

func (s *ValidateStep) validateLocked(ctx context.Context, st domain.ValidateStore, actor string, progress ProgressFunc) (ValidateSummary, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return ValidateSummary{}, err
	}
	prev := b.Status()
	runAt := s.now()
	if err := s.guard(ctx, st, b); err != nil {
		return ValidateSummary{}, err
	}
	progress.report(ctx, progressRead)
	vi, err := s.buildInput(ctx, st, b, runAt)
	if err != nil {
		return ValidateSummary{}, err
	}
	progress.report(ctx, progressCompute)
	res := validation.Validate(vi.in)
	progress.report(ctx, progressPersist)
	n, err := st.UpdateStdValidation(ctx, stdValidationUpdates(vi.in.Rows, res, vi.baseline))
	if err != nil {
		return ValidateSummary{}, err
	}
	sum := summarizeValidate(b, vi, res, runAt)
	sum.RowsUpdated = n
	sum.Acked = ackMatches(b, sum.WarningSetHash)
	if err := validateTransition(b, sum.Errors == 0 && (!sum.NeedsAck || sum.Acked), actor, runAt); err != nil {
		return ValidateSummary{}, err
	}
	sum.Validated = b.Status() == domain.StatusValidated
	sum.Status = string(b.Status())
	if err := setStepSummary(b, StepValidate, sum); err != nil {
		return ValidateSummary{}, err
	}
	b.SetErrorText("")
	if err := st.Save(ctx, b, prev, domain.Invalidation{}); err != nil {
		return ValidateSummary{}, err
	}
	return sum, nil
}

// guard checks the status and the G10 period lock (LIVE only).
func (s *ValidateStep) guard(ctx context.Context, st domain.ValidateStore, b *domain.Batch) error {
	if err := checkValidateStatus(b); err != nil {
		return err
	}
	if b.Mode() == domain.ModeShadow {
		return nil
	}
	locked, err := st.IsPeriodLocked(ctx, b.Period(), periodLockCalcType)
	if err != nil {
		return fmt.Errorf("check period lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("%w: %s %s", domain.ErrPeriodNotLocked, b.Period(), periodLockCalcType)
	}
	return nil
}

// checkValidateStatus accepts DERIVED and a re-validation of VALIDATED; a
// pushed (frozen) batch is refused.
func checkValidateStatus(b *domain.Batch) error {
	switch b.Status() {
	case domain.StatusDerived, domain.StatusValidated:
	default:
		return fmt.Errorf("%w: validate in %s", ErrStepNotAllowed, b.Status())
	}
	if b.IsFrozen() {
		return domain.ErrBatchFrozen
	}
	return nil
}

// validateTransition promotes a clean DERIVED batch and revokes a VALIDATED
// batch that no longer passes. No artifact is invalidated.
func validateTransition(b *domain.Batch, clean bool, actor string, at time.Time) error {
	switch {
	case clean && b.Status() == domain.StatusDerived:
		_, err := b.Transition(domain.StatusValidated, actor, at)
		return err
	case !clean && b.Status() == domain.StatusValidated:
		return b.RevokeValidation(actor, at)
	default:
		return nil
	}
}

// buildInput reads everything the validators need inside the transaction.
func (s *ValidateStep) buildInput(ctx context.Context, st domain.ValidateStore, b *domain.Batch, now time.Time) (validateInputs, error) {
	rows, err := st.ListStdRowsForValidation(ctx)
	if err != nil {
		return validateInputs{}, err
	}
	coverage, err := st.ListCoverage(ctx)
	if err != nil {
		return validateInputs{}, err
	}
	demand, err := st.ListDemand(ctx)
	if err != nil {
		return validateInputs{}, err
	}
	baseline, err := st.FindValidationBaseline(ctx, b.Period())
	if err != nil {
		return validateInputs{}, err
	}
	in := validation.Input{
		Period: b.Period(), Now: now, Rows: rows, Coverage: coverage, Demand: demand,
		PrevStd: map[domain.ErpKey]decimal.Decimal{}, CurrencyPolicy: currencyPolicy,
		DemandLoadedAt: b.DemandLoadedAt(), DemandMaxAge: s.maxAge,
	}
	if baseline != nil && baseline.Std != nil {
		in.PrevStd = baseline.Std
	}
	if err := s.readReferenceData(ctx, st, b.Period(), &in); err != nil {
		return validateInputs{}, err
	}
	vi := validateInputs{in: in, baseline: baseline}
	vi.in.PostedHeads, vi.probeError = s.probePosted(ctx, b.Period())
	return vi, nil
}

// readReferenceData fills the replica sets (V-06), source costs and the
// relabel flag (V-09).
func (s *ValidateStep) readReferenceData(ctx context.Context, st domain.ValidateStore, period string, in *validation.Input) error {
	items, shades, grades, costIDs := rowCodes(in.Rows)
	rc, err := st.ReplicaCodes(ctx, items, shades, grades)
	if err != nil {
		return err
	}
	in.Replica = &validation.ReplicaSets{Items: rc.Items, Shades: rc.Shades, Grades: rc.Grades}
	labels, err := st.SourceCostLabels(ctx, costIDs)
	if err != nil {
		return err
	}
	in.SourceCosts = make(map[int64]validation.SourceCost, len(labels))
	for id, l := range labels {
		in.SourceCosts[id] = validation.SourceCost{Currency: l.Currency, CostPerUnit: l.CostPerUnit}
	}
	if in.CurrencyRelabelApplied, err = st.CurrencyRelabelApplied(ctx, period); err != nil {
		return err
	}
	return nil
}

// probePosted runs the read-only V-10 posted-head probe. No prober or a
// probe error leaves the count nil, which V-10 reports as an error.
func (s *ValidateStep) probePosted(ctx context.Context, period string) (*int64, string) {
	if s.prober == nil {
		return nil, "posted-head probe not configured"
	}
	n, err := s.prober.ProbePosted(ctx, period)
	if err != nil {
		log.Warn().Err(err).Str("period", period).Msg("erp validate: posted-head probe failed")
		return nil, err.Error()
	}
	return &n, ""
}

// currencyPolicy adapts the P0-T10a interim rule to validation.CurrencyPolicy.
func currencyPolicy(period, item string, c validation.SourceCost) bool {
	return CoverageCurrencyOK(period, item, ActualCost{Currency: c.Currency, CostPerUnit: c.CostPerUnit})
}

// rowCodes returns the distinct item, shade and grade codes and AX cost ids
// of the rows, sorted.
func rowCodes(rows []domain.StdRow) (items, shades, grades []string, costIDs []int64) {
	is, ss, gs, cs := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[int64]struct{}{}
	for _, r := range rows {
		is[r.Key.ItemCode], ss[r.Key.ShadeCode], gs[r.Key.GradeCode] = struct{}{}, struct{}{}, struct{}{}
		if r.AxCostSysID != nil {
			cs[*r.AxCostSysID] = struct{}{}
		}
	}
	costIDs = make([]int64, 0, len(cs))
	for id := range cs {
		costIDs = append(costIDs, id)
	}
	sort.Slice(costIDs, func(i, j int) bool { return costIDs[i] < costIDs[j] })
	return sortedKeys(is), sortedKeys(ss), sortedKeys(gs), costIDs
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// stdValidationUpdates builds one update per row: Derive's issues kept, the
// row findings not already among them, and the baseline std.
func stdValidationUpdates(rows []domain.StdRow, res validation.Result, baseline *domain.ValidationBaseline) []domain.StdValidationUpdate {
	byKey := res.RowIssues()
	out := make([]domain.StdValidationUpdate, 0, len(rows))
	for _, r := range rows {
		u := domain.StdValidationUpdate{Key: r.Key, Derived: r.Issues, Validation: newIssues(r.Issues, byKey[r.Key])}
		if baseline != nil {
			if p, ok := baseline.Std[r.Key]; ok {
				u.PrevStd = decimal.NewNullDecimal(p)
			}
		}
		out = append(out, u)
	}
	return out
}

// newIssues returns the findings not equal (code, severity, message) to a
// derive-attached issue.
func newIssues(derived, found []domain.Issue) []domain.Issue {
	type sig struct {
		code domain.IssueCode
		sev  domain.Severity
		msg  string
	}
	seen := make(map[sig]struct{}, len(derived))
	for _, d := range derived {
		seen[sig{d.Code, d.Severity, d.Message}] = struct{}{}
	}
	out := make([]domain.Issue, 0, len(found))
	for _, f := range found {
		k := sig{f.Code, f.Severity, f.Message}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, f)
	}
	return out
}

func summarizeValidate(b *domain.Batch, vi validateInputs, res validation.Result, runAt time.Time) ValidateSummary {
	counts := res.Counts()
	sum := ValidateSummary{
		BatchID: b.ID(), Period: b.Period(), Rows: int64(len(vi.in.Rows)),
		Errors: counts[validation.SeverityError], Warnings: counts[validation.SeverityWarning],
		Infos: counts[validation.SeverityInfo], ByCode: map[string]int{},
		WarningSetHash: res.WarningSetHash(), NeedsAck: res.NeedsAck(), RuleHash: b.RuleHash(),
		DeriveRunAt: summaryDeriveRunAt(b.Summary()), ProbeError: vi.probeError, RunAt: runAt,
	}
	for c, n := range res.CountsByCode() {
		sum.ByCode[string(c)] = n
	}
	for _, f := range res.Findings {
		line := fmt.Sprintf("%s %s %s: %s", f.Code, f.Severity, f.Key, f.Message)
		switch {
		case f.Scope != validation.ScopeRow:
			if len(sum.Findings) < validateFindingSample {
				sum.Findings = append(sum.Findings, line)
			}
		case f.Severity != validation.SeverityInfo && len(sum.Issues) < validateFindingSample:
			sum.Issues = append(sum.Issues, line)
		}
	}
	sum.DryRun = buildDryRun(b, vi.in.Rows, vi.baseline)
	return sum
}

// buildDryRun builds the dry-run summary with the rule diff against the
// baseline's snapshot. A snapshot that cannot be parsed yields an empty
// diff and RuleDiffError (the dry run itself is still produced).
func buildDryRun(b *domain.Batch, rows []domain.StdRow, baseline *domain.ValidationBaseline) DryRunSummary {
	changes, diffErr := ruleDiff(baseline, b.RuleSnapshot())
	dr := BuildDryRunSummary(rows, baseline, changes, DryRunTopN)
	if diffErr != nil {
		dr.RuleDiffError = diffErr.Error()
	}
	return dr
}

// ruleDiff diffs the baseline's rule snapshot with cur (no baseline: every
// rule of cur is added).
func ruleDiff(baseline *domain.ValidationBaseline, cur []byte) ([]erprule.RuleChange, error) {
	var prev []byte
	if baseline != nil {
		prev = baseline.RuleSnapshot
	}
	return erprule.DiffSnapshots(prev, cur)
}

// ackMatches reports whether the batch holds a warnings ack (the aggregate
// stamp, cleared by a re-derive) recorded for exactly this warning set.
func ackMatches(b *domain.Batch, hash string) bool {
	if hash == "" || b.WarningsAck() == nil {
		return false
	}
	rec, ok := readAckRecord(b.Summary())
	return ok && rec.WarningSetHash == hash
}

// readSummaryEntry decodes ceib_summary[key] into v; false when absent or
// undecodable.
func readSummaryEntry(summary []byte, key string, v any) bool {
	if len(summary) == 0 {
		return false
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(summary, &m); err != nil {
		return false
	}
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// summaryDeriveRunAt returns ceib_summary.derive.run_at, or nil.
func summaryDeriveRunAt(summary []byte) *time.Time {
	var d struct {
		RunAt time.Time `json:"run_at"`
	}
	if !readSummaryEntry(summary, StepDerive, &d) || d.RunAt.IsZero() {
		return nil
	}
	return &d.RunAt
}
