package erpintegration

// coverage_step.go implements ComputeCoverage (plan-04 P3-T4 step 3; design
// Part 1 §2 step 6, §5.3; Part 2 §7 V-04, V-09, V-12). PG only.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// StepCoverage is the erp_integration subtype and the ceib_summary key.
const StepCoverage = "coverage"

// mbProductTypeCode is the cost_product_type code of masterbatch products
// (V-12: CMB items map only to MB; yarn never maps to MB).
const mbProductTypeCode = "MB"

// costStatusApproved is the only cst_product_cost status coverage accepts.
const costStatusApproved = "APPROVED"

// CoverageSummary is the coverage step summary (ceib_summary.coverage).
type CoverageSummary struct {
	BatchID   int64            `json:"batch_id"`
	Period    string           `json:"period"`
	Combos    int64            `json:"combos"`
	Counts    map[string]int64 `json:"counts"`
	Blocking  int64            `json:"blocking"`
	AllOK     bool             `json:"all_ok"`
	Status    string           `json:"batch_status"`
	QtyKg     string           `json:"qty_kg"`
	Digest    string           `json:"digest"`
	RunAt     time.Time        `json:"run_at"`
	Ambiguous int64            `json:"ambiguous"`
}

// CoverageStep computes cst_erp_coverage for a batch.
type CoverageStep struct {
	runner domain.BatchTxRunner
	source CoverageSource
	maxAge time.Duration
	now    func() time.Time
}

// NewCoverageStep builds the step. maxAge is erp_integration.demand_max_age
// (V-10 freshness; <= 0 disables the age check).
func NewCoverageStep(runner domain.BatchTxRunner, source CoverageSource, maxAge time.Duration) *CoverageStep {
	return &CoverageStep{runner: runner, source: source, maxAge: maxAge, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *CoverageStep) WithClock(now func() time.Time) *CoverageStep {
	s.now = now
	return s
}

// Run classifies every demanded (item, shade) combo under the G11 lock and
// replaces the coverage rows. The batch reaches COVERED only when every row
// is OK; otherwise it stays (or returns to) DEMAND_LOADED with the summary.
func (s *CoverageStep) Run(ctx context.Context, batchID int64, actor string, progress ProgressFunc) (CoverageSummary, error) {
	var sum CoverageSummary
	err := s.runner.RunLocked(ctx, batchID, func(ctx context.Context, st domain.BatchStore) error {
		b, err := st.GetForUpdate(ctx)
		if err != nil {
			return err
		}
		prev := b.Status()
		if prev != domain.StatusDemandLoaded && prev != domain.StatusCovered {
			return fmt.Errorf("%w: coverage in %s", ErrStepNotAllowed, prev)
		}
		runAt := s.now()
		if err := domain.CheckDemandFresh(b, runAt, s.maxAge); err != nil {
			return err
		}
		progress.report(ctx, progressRead)
		demand, err := st.ListDemand(ctx)
		if err != nil {
			return err
		}
		lines, err := s.classify(ctx, b, demand)
		if err != nil {
			return err
		}
		progress.report(ctx, progressPersist)
		if _, err := st.ReplaceCoverage(ctx, lines); err != nil {
			return err
		}
		inv, err := coverageTransition(b, lines, actor, runAt)
		if err != nil {
			return err
		}
		sum = summarizeCoverage(b, lines, runAt)
		if err := setStepSummary(b, StepCoverage, sum); err != nil {
			return err
		}
		b.SetErrorText("")
		return st.Save(ctx, b, prev, inv)
	})
	if err != nil {
		return CoverageSummary{}, err
	}
	return sum, nil
}

func (s *CoverageStep) classify(ctx context.Context, b *domain.Batch, demand []domain.DemandLine) ([]domain.CoverageLine, error) {
	combos := GroupDemandCombos(b.ID(), demand)
	keys := make([]ErpProductKey, len(combos))
	for i, c := range combos {
		keys[i] = NewErpProductKey(c.ItemCode, c.ShadeCode)
	}
	products, err := s.source.ResolveProducts(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("resolve ERP products: %w", err)
	}
	var ids []int64
	for _, k := range keys {
		if c := products[k]; len(c) == 1 {
			ids = append(ids, c[0].SysID)
		}
	}
	costs := map[int64]ActualCost{}
	if len(ids) > 0 {
		if costs, err = s.source.ActualCosts(ctx, b.Period(), ids); err != nil {
			return nil, fmt.Errorf("load ACTUAL costs: %w", err)
		}
	}
	out := make([]domain.CoverageLine, len(combos))
	for i, c := range combos {
		out[i] = ClassifyCoverage(b.Period(), c, products[keys[i]], costs)
		if err := out[i].Validate(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// coverageTransition applies the §5.3 coverage outcome: all OK -> COVERED
// (from DEMAND_LOADED or re-coverage from COVERED); gaps -> stay
// DEMAND_LOADED, or reopen COVERED back to DEMAND_LOADED (demand kept).
func coverageTransition(b *domain.Batch, lines []domain.CoverageLine, actor string, at time.Time) (domain.Invalidation, error) {
	if coverageAllOK(lines) {
		return b.Transition(domain.StatusCovered, actor, at)
	}
	if b.Status() == domain.StatusCovered {
		return b.ReopenCoverage(actor, at)
	}
	return domain.Invalidation{}, nil
}

func coverageAllOK(lines []domain.CoverageLine) bool {
	return countCoverage(lines).AllOK()
}

func countCoverage(lines []domain.CoverageLine) domain.CoverageCounts {
	c := domain.CoverageCounts{}
	for _, l := range lines {
		c[l.Status]++
	}
	return c
}

// GroupDemandCombos folds the demand lines into one coverage line skeleton
// per (item, shade) combo (normalized D-LINK key; the first raw spelling is
// kept), aggregating the ADJ grade codes and qty. The result is sorted by
// (item, shade) and carries status "" until classified.
func GroupDemandCombos(batchID int64, demand []domain.DemandLine) []domain.CoverageLine {
	type acc struct {
		line   domain.CoverageLine
		grades map[string]struct{}
	}
	byKey := map[ErpProductKey]*acc{}
	var order []ErpProductKey
	for _, d := range demand {
		k := NewErpProductKey(d.ItemCode, d.ShadeCode)
		a, ok := byKey[k]
		if !ok {
			a = &acc{
				line: domain.CoverageLine{
					BatchID: batchID, Kind: domain.ItemKindForCode(d.ItemCode),
					ItemCode: strings.TrimSpace(d.ItemCode), ShadeCode: strings.TrimSpace(d.ShadeCode),
					QtyKg: zeroDecimal(),
				},
				grades: map[string]struct{}{},
			}
			byKey[k] = a
			order = append(order, k)
		}
		a.line.QtyKg = a.line.QtyKg.Add(d.QtyKg)
		a.grades[strings.TrimSpace(d.GradeCode)] = struct{}{}
	}
	out := make([]domain.CoverageLine, 0, len(order))
	for _, k := range order {
		a := byKey[k]
		grades := make([]string, 0, len(a.grades))
		for g := range a.grades {
			grades = append(grades, g)
		}
		sort.Strings(grades)
		a.line.GradeCodes = grades
		out = append(out, a.line)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ItemCode != out[j].ItemCode {
			return out[i].ItemCode < out[j].ItemCode
		}
		return out[i].ShadeCode < out[j].ShadeCode
	})
	return out
}

// ClassifyCoverage decides the status of one combo (pure):
//
//   - 0 active AX products for the key            -> NO_MAPPING
//   - > 1 (V-04 ambiguous; never guessed)          -> DUP_MAPPING + candidates
//   - V-12: CMB item on a non-MB product, or a yarn item on an MB product -> INVALID
//   - no active ACTUAL cost for the period         -> NO_COST
//   - cost not APPROVED                            -> NOT_APPROVED
//   - V-09 currency rule fails (CoverageCurrencyOK) -> NOT_USD
//   - otherwise                                    -> OK (product + cost ids set)
func ClassifyCoverage(period string, combo domain.CoverageLine, candidates []ProductCandidate, costs map[int64]ActualCost) domain.CoverageLine {
	l := combo
	l.ProductSysID, l.CostID, l.CostVersion, l.Candidates = nil, nil, nil, nil
	switch len(candidates) {
	case 0:
		l.Status = domain.CoverageNoMapping
		l.Reason = "no active AX product with cpm_erp_item_code + cpm_shade_code = " + comboLabel(l)
		return l
	case 1:
	default:
		l.Status = domain.CoverageDupMapping
		l.Reason = fmt.Sprintf("V-04 ambiguous: %d active AX products for %s (product_sys_id %s)",
			len(candidates), comboLabel(l), joinSysIDs(candidates))
		l.Candidates = candidatesJSON(candidates)
		return l
	}
	p := candidates[0]
	pid := p.SysID
	l.ProductSysID = &pid
	isMBType := strings.EqualFold(strings.TrimSpace(p.TypeCode), mbProductTypeCode)
	if l.Kind == domain.ItemKindMB && !isMBType {
		l.Status = domain.CoverageInvalid
		l.Reason = fmt.Sprintf("V-12: CMB item %s linked to product %d of type %s (MB required)", l.ItemCode, pid, p.TypeCode)
		return l
	}
	if l.Kind == domain.ItemKindYarn && isMBType {
		l.Status = domain.CoverageInvalid
		l.Reason = fmt.Sprintf("V-12: yarn item %s linked to MB product %d", l.ItemCode, pid)
		return l
	}
	c, ok := costs[pid]
	if !ok {
		l.Status = domain.CoverageNoCost
		l.Reason = fmt.Sprintf("no active ACTUAL cost for product %d in %s", pid, period)
		return l
	}
	cid, ver := c.CostID, c.Version
	l.CostID, l.CostVersion = &cid, &ver
	if !strings.EqualFold(strings.TrimSpace(c.Status), costStatusApproved) {
		l.Status = domain.CoverageNotApproved
		l.Reason = fmt.Sprintf("ACTUAL cost %d is %s (APPROVED required)", cid, c.Status)
		return l
	}
	if !CoverageCurrencyOK(period, l.ItemCode, c) {
		l.Status = domain.CoverageNotUSD
		l.Reason = fmt.Sprintf("V-09: cost %d currency %s value %s fails the USD rule for %s",
			cid, c.Currency, c.CostPerUnit.String(), period)
		return l
	}
	l.Status = domain.CoverageOK
	l.Reason = ""
	return l
}

// CoverageCurrencyOK is V-09 for coverage: USD always passes; an IDR label
// passes only under the interim rule (not an outlier, design §7.3). A period
// excluded from the 000557 relabel (202605, F-4/F-10) fails closed: it needs
// a USD label AND a non-outlier value.
func CoverageCurrencyOK(period, erpItemCode string, c ActualCost) bool {
	outlier := IsCurrencyOutlier(erpItemCode, c.CostPerUnit)
	if _, excluded := RelabelExcludedPeriods[period]; excluded {
		return strings.EqualFold(strings.TrimSpace(c.Currency), CurrencyUSD) && !outlier
	}
	return IsCurrencyAcceptable(c.Currency, outlier)
}

func comboLabel(l domain.CoverageLine) string {
	return fmt.Sprintf("%s/%s", l.ItemCode, l.ShadeCode)
}

func sortedSysIDs(cs []ProductCandidate) []int64 {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.SysID
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func joinSysIDs(cs []ProductCandidate) string {
	ids := sortedSysIDs(cs)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

// candidateJSON is one entry of cec_candidates for a DUP_MAPPING combo.
type candidateJSON struct {
	ProductSysID int64  `json:"product_sys_id"`
	ProductCode  string `json:"product_code,omitempty"`
	TypeCode     string `json:"type_code,omitempty"`
}

func candidatesJSON(cs []ProductCandidate) []byte {
	sorted := make([]ProductCandidate, len(cs))
	copy(sorted, cs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SysID < sorted[j].SysID })
	out := struct {
		Reason     string          `json:"reason"`
		Candidates []candidateJSON `json:"candidates"`
	}{Reason: "V-04 ambiguous"}
	for _, c := range sorted {
		out.Candidates = append(out.Candidates, candidateJSON{ProductSysID: c.SysID, ProductCode: c.ProductCode, TypeCode: c.TypeCode})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return raw
}

func summarizeCoverage(b *domain.Batch, lines []domain.CoverageLine, runAt time.Time) CoverageSummary {
	counts := countCoverage(lines)
	sum := CoverageSummary{
		BatchID: b.ID(), Period: b.Period(), Combos: counts.Total(), Counts: map[string]int64{},
		AllOK: counts.AllOK(), Status: string(b.Status()), RunAt: runAt,
		Digest: CoverageDigest(lines), Ambiguous: counts[domain.CoverageDupMapping],
	}
	for _, st := range domain.AllCoverageStatuses() {
		sum.Counts[string(st)] = counts[st]
		if st.IsBlocking() {
			sum.Blocking += counts[st]
		}
	}
	qty := zeroDecimal()
	for _, l := range lines {
		qty = qty.Add(l.QtyKg)
	}
	sum.QtyKg = qty.String()
	return sum
}

// CoverageDigest is the SHA-256 hex over the canonical coverage lines
// (item, shade, grades, product, cost, status, qty). Unchanged inputs give an
// identical digest (AC-05).
func CoverageDigest(lines []domain.CoverageLine) string {
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		parts = append(parts, strings.Join([]string{
			string(l.Kind), l.ItemCode, l.ShadeCode, strings.Join(l.GradeCodes, ","),
			optInt64(l.ProductSysID), optInt64(l.CostID), optInt32(l.CostVersion),
			string(l.Status), l.QtyKg.String(), string(l.Candidates),
		}, "\x1f"))
	}
	sort.Strings(parts)
	return sha256Hex(strings.Join(parts, "\x1e"))
}

func optInt64(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func optInt32(v *int32) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(int64(*v), 10)
}

func zeroDecimal() decimal.Decimal { return decimal.Zero }

func nullDecString(d decimal.NullDecimal) string {
	if !d.Valid {
		return "\x00"
	}
	return d.Decimal.String()
}
