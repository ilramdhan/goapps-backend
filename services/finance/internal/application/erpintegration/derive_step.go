package erpintegration

// derive_step.go implements the Derive step (plan-05 P4-T4b; design Part 1
// §2 step 7, §5.3; Part 2 §6, §6.6, §9.2). PG only: it reads the batch's
// coverage, the AX cost inputs and the active rule set, values every
// (item, grade, shade) with the pure engine and replaces cst_erp_std_cost
// in the G11-locked transaction.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// StepDerive is the erp_integration subtype and the ceib_summary key.
const StepDerive = "derive"

// Derive step errors.
var (
	// ErrRuleSetLoaderNotConfigured is returned when no rule-set loader is
	// wired: the derive fails closed.
	ErrRuleSetLoaderNotConfigured = errors.New("erpintegration: rule set loader not configured")
	// ErrDeriveStoreUnsupported is returned when the batch runner's store
	// does not implement domain.DeriveStore.
	ErrDeriveStoreUnsupported = errors.New("erpintegration: batch store does not support the derive step")
)

// DeriveSummary is the derive step summary (ceib_summary.derive and job
// result_summary). RowsMD5 + DigestVersion are the persisted half of the
// control digest (no dedicated column; ledger P4-T4a).
type DeriveSummary struct {
	BatchID       int64            `json:"batch_id"`
	Period        string           `json:"period"`
	Rows          int64            `json:"rows"`
	Counts        map[string]int64 `json:"counts"`
	ErrorRows     int64            `json:"error_rows"`
	Errors        int64            `json:"errors"`
	Warnings      int64            `json:"warnings"`
	RuleHash      string           `json:"rule_hash"`
	RowCount      int64            `json:"row_count"`
	SumStd        string           `json:"sum_std"`
	SumConv       string           `json:"sum_conv"`
	SumPvl        string           `json:"sum_pvl"`
	RowsMD5       string           `json:"rows_md5"`
	DigestVersion int              `json:"digest_version"`
	Derived       bool             `json:"derived"`
	Status        string           `json:"batch_status"`
	RunAt         time.Time        `json:"run_at"`
	// Issues is a bounded sample of the error issues (V-08 worklist); the
	// full list lives on the std rows (cesc_validation).
	Issues []string `json:"issues,omitempty"`
}

// deriveIssueSample bounds DeriveSummary.Issues.
const deriveIssueSample = 50

// DeriveStep derives the std cost rows of a batch.
type DeriveStep struct {
	runner domain.BatchTxRunner
	rules  erprule.RuleSetLoader
	maxAge time.Duration
	now    func() time.Time
}

// NewDeriveStep builds the step. rules nil makes every run fail with
// ErrRuleSetLoaderNotConfigured. maxAge is erp_integration.demand_max_age
// (V-10 freshness; <= 0 disables the age check).
func NewDeriveStep(runner domain.BatchTxRunner, rules erprule.RuleSetLoader, maxAge time.Duration) *DeriveStep {
	return &DeriveStep{runner: runner, rules: rules, maxAge: maxAge, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *DeriveStep) WithClock(now func() time.Time) *DeriveStep {
	s.now = now
	return s
}

// Run derives under the G11 lock. Guards: status COVERED (or a re-derive
// from DERIVED / VALIDATED, never after push), fresh demand (V-10) and, for
// a LIVE batch, the (P, ACTUAL) period lock (G10, FOR SHARE in the tx). A
// SHADOW batch (PG-only backtest) skips G10 like it does at create.
//
// The std rows are always replaced so the result (including failing rows)
// is visible. With no error row the batch moves to DERIVED and stores the
// rule snapshot, hash and control totals; with error rows (V-08, V-03,
// V-06) it stays COVERED (a re-derive from DERIVED/VALIDATED is reopened to
// COVERED) and the snapshot is not stored.
func (s *DeriveStep) Run(ctx context.Context, batchID int64, actor string, progress ProgressFunc) (DeriveSummary, error) {
	if s.rules == nil {
		return DeriveSummary{}, ErrRuleSetLoaderNotConfigured
	}
	var sum DeriveSummary
	err := s.runner.RunLocked(ctx, batchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.DeriveStore)
		if !ok {
			return ErrDeriveStoreUnsupported
		}
		var err error
		sum, err = s.deriveLocked(ctx, st, actor, progress)
		return err
	})
	if err != nil {
		return DeriveSummary{}, err
	}
	return sum, nil
}

// deriveLocked is the body of Run inside the G11-locked transaction.
func (s *DeriveStep) deriveLocked(ctx context.Context, st domain.DeriveStore, actor string, progress ProgressFunc) (DeriveSummary, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return DeriveSummary{}, err
	}
	prev := b.Status()
	runAt := s.now()
	if err := s.guard(ctx, st, b, runAt); err != nil {
		return DeriveSummary{}, err
	}
	progress.report(ctx, progressRead)
	inputs, err := s.buildInputs(ctx, st, b)
	if err != nil {
		return DeriveSummary{}, err
	}
	rs, err := s.rules.LoadRuleSet(ctx)
	if err != nil {
		return DeriveSummary{}, fmt.Errorf("load rule set: %w", err)
	}
	if rs == nil {
		return DeriveSummary{}, fmt.Errorf("load rule set: %w", ErrRuleSetLoaderNotConfigured)
	}
	progress.report(ctx, progressCompute)
	rows, issues := domain.Derive(inputs, rs)
	digest, err := domain.ComputeStdDigest(rows)
	if err != nil {
		return DeriveSummary{}, err
	}
	progress.report(ctx, progressPersist)
	if _, err := st.ReplaceStdRows(ctx, b.Period(), rows); err != nil {
		return DeriveSummary{}, err
	}
	sum := summarizeDerive(b, rows, issues, rs.Hash(), digest, runAt)
	inv, err := deriveTransition(b, prev, sum.ErrorRows == 0, rs, digest, actor, runAt)
	if err != nil {
		return DeriveSummary{}, err
	}
	sum.Derived = b.Status() == domain.StatusDerived
	sum.Status = string(b.Status())
	if err := setStepSummary(b, StepDerive, sum); err != nil {
		return DeriveSummary{}, err
	}
	b.SetErrorText("")
	if err := st.Save(ctx, b, prev, inv); err != nil {
		return DeriveSummary{}, err
	}
	return sum, nil
}

// guard checks the status, freshness and period-lock preconditions.
func (s *DeriveStep) guard(ctx context.Context, st domain.DeriveStore, b *domain.Batch, now time.Time) error {
	if err := checkDeriveStatus(b); err != nil {
		return err
	}
	if err := domain.CheckDemandFresh(b, now, s.maxAge); err != nil {
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

// checkDeriveStatus accepts COVERED and a re-derive from DERIVED /
// VALIDATED; a pushed (frozen) batch is refused.
func checkDeriveStatus(b *domain.Batch) error {
	switch b.Status() {
	case domain.StatusCovered, domain.StatusDerived, domain.StatusValidated:
	default:
		return fmt.Errorf("%w: derive in %s", ErrStepNotAllowed, b.Status())
	}
	if b.IsFrozen() {
		return domain.ErrBatchFrozen
	}
	return nil
}

// deriveTransition applies the derive outcome. clean -> DERIVED + snapshot,
// hash and totals; otherwise COVERED stays COVERED (no invalidation: the std
// rows were just replaced) and DERIVED/VALIDATED is reopened to COVERED.
func deriveTransition(b *domain.Batch, prev domain.BatchStatus, clean bool, rs *erprule.RuleSet, digest domain.StdDigest, actor string, at time.Time) (domain.Invalidation, error) {
	if !clean {
		if prev == domain.StatusCovered {
			return domain.Invalidation{}, nil
		}
		return b.ReopenDerivation(actor, at)
	}
	inv, err := b.Transition(domain.StatusDerived, actor, at)
	if err != nil {
		return domain.Invalidation{}, err
	}
	if err := b.SetDerivation(rs.Canonical(), rs.Hash(), digest.Totals, actor, at); err != nil {
		return domain.Invalidation{}, err
	}
	return inv, nil
}

// buildInputs reads coverage, AX inputs, item and shade names and expands
// the coverage lines into derive inputs (ExpandCoverage).
func (s *DeriveStep) buildInputs(ctx context.Context, st domain.DeriveStore, b *domain.Batch) ([]domain.DeriveInput, error) {
	lines, err := st.ListCoverage(ctx)
	if err != nil {
		return nil, err
	}
	var costIDs, shadeCodes = []int64{}, []string{}
	for _, l := range lines {
		if l.Status == domain.CoverageOK && l.CostID != nil {
			costIDs = append(costIDs, *l.CostID)
		}
		shadeCodes = append(shadeCodes, l.ShadeCode)
	}
	ax, err := st.LoadAxComponents(ctx, b.Period(), costIDs)
	if err != nil {
		return nil, err
	}
	shades, err := st.ShadeNames(ctx, shadeCodes)
	if err != nil {
		return nil, err
	}
	demand, err := st.ListDemand(ctx)
	if err != nil {
		return nil, err
	}
	return ExpandCoverage(lines, ax, deriveItemNames(demand), shades), nil
}

// deriveItemNames returns the first non-empty demand item name per trimmed,
// upper-cased item code.
func deriveItemNames(demand []domain.DemandLine) map[string]string {
	out := map[string]string{}
	for _, d := range demand {
		k := normCode(d.ItemCode)
		if _, ok := out[k]; ok {
			continue
		}
		if n := strings.TrimSpace(d.ItemName); n != "" {
			out[k] = n
		}
	}
	return out
}

// ExpandCoverage turns coverage lines into derive inputs (decided expansion
// rule, ledger 2026-09-30; design §6.2, legacy §1.2/1.3):
//
//   - yarn: one AX row per (item, shade) plus one derived row per distinct
//     non-AX grade in the line's ADJ grade codes;
//   - MB: one grade-A row, plus a row per other distinct MB grade (the
//     engine values those NO_RULE: MB is valued for grade A only);
//   - a line whose coverage is not OK, or whose cost id has no usable AX
//     inputs (no longer APPROVED, no RM cost), gets Ax = nil (NO_AX).
//
// Inputs are sorted by (item, grade, shade) byte-wise; a key produced twice
// is kept once. names and shades are keyed by the trimmed, upper-cased code.
func ExpandCoverage(lines []domain.CoverageLine, ax map[int64]domain.AxComponents, names, shades map[string]string) []domain.DeriveInput {
	seen := map[domain.ErpKey]struct{}{}
	out := make([]domain.DeriveInput, 0, len(lines)*2)
	for _, l := range lines {
		axp := lineAx(l, ax)
		kind := l.Kind
		if kind == "" {
			kind = domain.ItemKindForCode(l.ItemCode)
		}
		for _, g := range lineGrades(kind, l.GradeCodes) {
			k := domain.ErpKey{ItemCode: l.ItemCode, GradeCode: g, ShadeCode: l.ShadeCode}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, domain.DeriveInput{
				Key: k, Kind: kind, Ax: axp,
				ItemName:  names[normCode(l.ItemCode)],
				ShadeName: shades[normCode(l.ShadeCode)],
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return lessKey(out[i].Key, out[j].Key) })
	return out
}

// lineAx returns a copy of the line's AX inputs, or nil (NO_AX) when the
// coverage is not OK or the cost id has no usable inputs.
func lineAx(l domain.CoverageLine, ax map[int64]domain.AxComponents) *domain.AxComponents {
	if l.Status != domain.CoverageOK || l.CostID == nil {
		return nil
	}
	a, ok := ax[*l.CostID]
	if !ok {
		return nil
	}
	return &a
}

// lineGrades returns the base grade (AX for yarn, A for MB) followed by the
// trimmed, non-empty demand grades that differ from it.
func lineGrades(kind domain.ItemKind, codes []string) []string {
	base := domain.GradeAX
	if kind == domain.ItemKindMB {
		base = domain.GradeMBA
	}
	grades := []string{base}
	for _, g := range codes {
		if g = strings.TrimSpace(g); g != "" && g != base {
			grades = append(grades, g)
		}
	}
	return grades
}

func lessKey(a, b domain.ErpKey) bool {
	if a.ItemCode != b.ItemCode {
		return a.ItemCode < b.ItemCode
	}
	if a.GradeCode != b.GradeCode {
		return a.GradeCode < b.GradeCode
	}
	return a.ShadeCode < b.ShadeCode
}

func normCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func summarizeDerive(b *domain.Batch, rows []domain.StdRow, issues []domain.Issue, ruleHash string, digest domain.StdDigest, runAt time.Time) DeriveSummary {
	sum := DeriveSummary{
		BatchID: b.ID(), Period: b.Period(), Rows: int64(len(rows)), Counts: map[string]int64{},
		RuleHash: ruleHash, RowCount: digest.Totals.RowCount(),
		SumStd:  digest.Totals.SumStd().StringFixed(domain.ScaleR5),
		SumConv: digest.Totals.SumConv().StringFixed(domain.ScaleR5),
		SumPvl:  digest.Totals.SumPvl().StringFixed(domain.ScaleR5),
		RowsMD5: digest.RowsMD5, DigestVersion: domain.StdDigestVersion, RunAt: runAt,
	}
	for _, r := range rows {
		sum.Counts[string(r.Status)]++
		if r.HasErrors() {
			sum.ErrorRows++
		}
	}
	for _, is := range issues {
		if is.Severity == domain.SeverityError {
			sum.Errors++
			if len(sum.Issues) < deriveIssueSample {
				sum.Issues = append(sum.Issues, fmt.Sprintf("%s %s: %s", is.Code, is.Key, is.Message))
			}
		} else {
			sum.Warnings++
		}
	}
	return sum
}
