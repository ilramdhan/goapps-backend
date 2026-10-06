package erpintegration

// load_demand_step.go implements the LoadDemand step (plan-04 P3-T4 step 2;
// design Part 1 §2 step 5, §5.3). Oracle is only read, through the
// ErpDemandReader built on the ReadOnlyQuerier (SELECT only).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// StepLoadDemand is the erp_integration subtype and the ceib_summary key.
const StepLoadDemand = "load_demand"

// LoadDemandSummary is the load_demand step summary (ceib_summary.load_demand
// and job result_summary).
type LoadDemandSummary struct {
	BatchID     int64     `json:"batch_id"`
	Period      string    `json:"period"`
	Rows        int64     `json:"rows"`
	Items       int       `json:"items"`
	QtyKg       string    `json:"qty_kg"`
	Digest      string    `json:"digest"`
	LoadedAt    time.Time `json:"loaded_at"`
	PostedHeads *int64    `json:"posted_heads,omitempty"`
	// Warnings: V-10 posted > 0 is a warning at load and blocking at validate
	// (design §5.3).
	Warnings []string `json:"warnings,omitempty"`
}

// LoadDemandStep replaces the batch's demand snapshot.
type LoadDemandStep struct {
	runner domain.BatchTxRunner
	reader domain.ErpDemandReader
	prober domain.ErpAdjHeadProber
	now    func() time.Time
}

// NewLoadDemandStep builds the step. reader nil (Oracle absent) makes every
// run fail with ErrDemandReaderNotConfigured; prober may be nil (the posted
// warning is then taken from the demand rows).
func NewLoadDemandStep(runner domain.BatchTxRunner, reader domain.ErpDemandReader, prober domain.ErpAdjHeadProber) *LoadDemandStep {
	return &LoadDemandStep{runner: runner, reader: reader, prober: prober, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *LoadDemandStep) WithClock(now func() time.Time) *LoadDemandStep {
	s.now = now
	return s
}

// Run loads the demand under the G11 advisory lock: it re-reads the batch
// FOR UPDATE, SELECTs the ADJ demand, replaces cst_erp_adj_demand, stamps
// demand_loaded_at and moves the batch to DEMAND_LOADED (a reload from
// COVERED/DERIVED/VALIDATED clears coverage and std rows in the same tx).
func (s *LoadDemandStep) Run(ctx context.Context, batchID int64, actor string, progress ProgressFunc) (LoadDemandSummary, error) {
	if s.reader == nil {
		return LoadDemandSummary{}, ErrDemandReaderNotConfigured
	}
	var sum LoadDemandSummary
	err := s.runner.RunLocked(ctx, batchID, func(ctx context.Context, st domain.BatchStore) error {
		b, err := st.GetForUpdate(ctx)
		if err != nil {
			return err
		}
		prev := b.Status()
		if !domain.CanTransition(prev, domain.StatusDemandLoaded) {
			return fmt.Errorf("%w: load demand in %s", ErrStepNotAllowed, prev)
		}
		progress.report(ctx, progressRead)
		rows, err := s.reader.LoadAdjDemand(ctx, b.Period())
		if err != nil {
			return fmt.Errorf("read ADJ demand: %w", err)
		}
		loadedAt := s.now()
		lines, err := buildDemandLines(batchID, rows, loadedAt)
		if err != nil {
			return err
		}
		progress.report(ctx, progressCompute)
		sum = summarizeDemand(b, lines, loadedAt)
		s.postedWarning(ctx, b.Period(), rows, &sum)

		progress.report(ctx, progressPersist)
		n, err := st.ReplaceDemand(ctx, lines)
		if err != nil {
			return err
		}
		sum.Rows = n
		inv, err := b.Transition(domain.StatusDemandLoaded, actor, loadedAt)
		if err != nil {
			return err
		}
		if err := setStepSummary(b, StepLoadDemand, sum); err != nil {
			return err
		}
		b.SetErrorText("")
		return st.Save(ctx, b, prev, inv)
	})
	if err != nil {
		return LoadDemandSummary{}, err
	}
	return sum, nil
}

// postedWarning records the V-10 posted count as a warning (blocking only at
// validate). The live probe is used when wired; its failure is a warning too
// and falls back to the posted counts of the demand rows.
func (s *LoadDemandStep) postedWarning(ctx context.Context, period string, rows []domain.ErpDemandRow, sum *LoadDemandSummary) {
	var posted int64
	if s.prober != nil {
		n, err := s.prober.ProbePosted(ctx, period)
		if err == nil {
			posted = n
			sum.PostedHeads = &posted
		} else {
			sum.Warnings = append(sum.Warnings, "V-10 posted probe failed: "+err.Error())
		}
	}
	if sum.PostedHeads == nil {
		for _, r := range rows {
			if r.PostedItems > 0 {
				posted++
			}
		}
	}
	if posted > 0 {
		sum.Warnings = append(sum.Warnings, fmt.Sprintf("V-10: %d posted ADJ head(s)/line(s) for %s; validate will block", posted, period))
	}
}

func buildDemandLines(batchID int64, rows []domain.ErpDemandRow, loadedAt time.Time) ([]domain.DemandLine, error) {
	lines := make([]domain.DemandLine, 0, len(rows))
	seen := make(map[domain.DemandKey]struct{}, len(rows))
	for _, r := range rows {
		l, err := domain.NewDemandLineFromErpRow(batchID, r, loadedAt)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[l.Key()]; dup {
			return nil, fmt.Errorf("%w: duplicate key %s/%s/%s/%s", domain.ErrInvalidDemandLine,
				l.TxnCode, l.ItemCode, l.GradeCode, l.ShadeCode)
		}
		seen[l.Key()] = struct{}{}
		lines = append(lines, l)
	}
	sortDemandLines(lines)
	return lines, nil
}

func sortDemandLines(lines []domain.DemandLine) {
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i].Key(), lines[j].Key()
		if a.TxnCode != b.TxnCode {
			return a.TxnCode < b.TxnCode
		}
		if a.ItemCode != b.ItemCode {
			return a.ItemCode < b.ItemCode
		}
		if a.GradeCode != b.GradeCode {
			return a.GradeCode < b.GradeCode
		}
		return a.ShadeCode < b.ShadeCode
	})
}

func summarizeDemand(b *domain.Batch, lines []domain.DemandLine, loadedAt time.Time) LoadDemandSummary {
	items := make(map[string]struct{}, len(lines))
	qty := zeroDecimal()
	for _, l := range lines {
		items[l.ItemCode] = struct{}{}
		qty = qty.Add(l.QtyKg)
	}
	return LoadDemandSummary{
		BatchID: b.ID(), Period: b.Period(), Rows: int64(len(lines)), Items: len(items),
		QtyKg: qty.String(), Digest: DemandDigest(lines), LoadedAt: loadedAt,
	}
}

// DemandDigest is the SHA-256 hex over the canonical demand lines (sorted by
// the uk_ced key; every sourced column, decimals in canonical string form).
// loaded_at and batch_id are excluded, so unchanged ERP data gives an
// identical digest on every reload (AC-05).
func DemandDigest(lines []domain.DemandLine) string {
	sorted := make([]domain.DemandLine, len(lines))
	copy(sorted, lines)
	sortDemandLines(sorted)
	h := sha256.New()
	for _, l := range sorted {
		fields := []string{
			l.Period, l.TxnCode, string(l.Kind), l.ItemCode, l.ItemName, l.GradeCode, l.ShadeCode,
			strconv.FormatInt(l.ItemCount, 10), strconv.FormatInt(l.RateVariants, 10),
			l.QtyKg.String(), nullDecString(l.MinRate), nullDecString(l.MaxRate), nullDecString(l.AdjVal),
			strconv.FormatInt(l.ApprovedItems, 10), strconv.FormatInt(l.PostedItems, 10),
			l.GoappsBatch, l.GoappsSource,
		}
		_, _ = h.Write([]byte(strings.Join(fields, "\x1f")))
		_, _ = h.Write([]byte{'\x1e'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func setStepSummary(b *domain.Batch, step string, v any) error {
	merged, err := mergeStepSummary(b.Summary(), step, v)
	if err != nil {
		return err
	}
	b.SetSummary(merged)
	return nil
}
