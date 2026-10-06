package erpintegration

// recon_step.go implements the recon read-back step (plan-06 P5-T6 step 1;
// design Part 1 §2 step 12, §5.3 VALUATED <-> RECONCILED, C-4, C-12; Part 3
// §14 "Unknown outcome"; PRD recon §4 / §4a / §7). It is read-only towards
// Oracle: the header, cost rows and ADJ aggregates are SELECTed through the
// ReadOnlyGuard reader. Nothing is ever written to Oracle and no writer is
// involved, so no write flag or writer mode gates it.
//
// Under the G11 lock:
//
//  1. read back the GoApps batch (CST_GOAPPS_STD_*) and the period's ADJ;
//  2. resolve every STARTED / UNKNOWN call-log row of the batch from that
//     evidence (SUCCESS / FAILED in the PG call log only; never a re-call);
//  3. on a VALUATED / RECONCILED batch: compare totals + md5 (PG vs Oracle),
//     classify each OK std row MATCH / DIFF / NOT_IN_ADJ, count NOT_COVERED,
//     store cesc_erp_* / cesc_recon_status and move to RECONCILED only when
//     everything matches (a RECONCILED batch with a new DIFF goes back to
//     VALUATED);
//  4. on a PUSHED / FAILED batch (an in-flight or failed write): resolve
//     only, the batch status is untouched.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// StepRecon is the erp_integration subtype and the ceib_summary key.
const StepRecon = "recon"

// Recon errors.
var (
	// ErrReconReaderNotConfigured is returned when no read-only Oracle recon
	// reader is wired (Oracle absent): recon fails closed.
	ErrReconReaderNotConfigured = errors.New("erpintegration: oracle recon reader not configured")
	// ErrReconStoreUnsupported is returned when the batch store cannot run
	// the recon step.
	ErrReconStoreUnsupported = errors.New("erpintegration: batch store does not support the recon step")
)

// ReconSummary is ceib_summary.recon and the job result.
type ReconSummary struct {
	BatchID      int64              `json:"batch_id"`
	Period       string             `json:"period"`
	Mode         string             `json:"mode"` // full | resolve_only
	OracleFound  bool               `json:"oracle_found"`
	OracleStatus string             `json:"oracle_status,omitempty"`
	TotalsEqual  bool               `json:"totals_equal"`
	MD5Equal     bool               `json:"md5_equal"`
	RuleHashOK   bool               `json:"rule_hash_equal"`
	PgRowCount   int64              `json:"pg_row_count"`
	OraRowCount  int64              `json:"oracle_row_count"`
	PgRowsMD5    string             `json:"pg_rows_md5,omitempty"`
	OraRowsMD5   string             `json:"oracle_rows_md5,omitempty"`
	Counts       domain.ReconCounts `json:"counts"`
	NotCovered   []string           `json:"not_covered,omitempty"` // first reconListMax keys
	RowsWritten  int64              `json:"rows_written"`
	Calls        []CallResolution   `json:"call_resolutions,omitempty"`
	Unresolved   int                `json:"unresolved_calls"`
	Reconciled   bool               `json:"reconciled"`
	Reasons      []string           `json:"reasons,omitempty"`
	Status       string             `json:"batch_status"`
	RunAt        time.Time          `json:"run_at"`
}

// reconListMax bounds the key list kept in the summary (recon §7 lists the
// rest in the export).
const reconListMax = 50

// Recon summary modes.
const (
	reconModeFull        = "full"
	reconModeResolveOnly = "resolve_only"
)

// ReconStep runs the recon read-back.
type ReconStep struct {
	runner   domain.BatchTxRunner
	reader   domain.ErpReconReader
	resolver domain.OracleCallResolver
	now      func() time.Time
}

// NewReconStep builds the step. reader nil fails closed (Oracle absent);
// resolver nil fails closed too (an unresolved call can never be ignored).
func NewReconStep(runner domain.BatchTxRunner, reader domain.ErpReconReader, resolver domain.OracleCallResolver) *ReconStep {
	return &ReconStep{runner: runner, reader: reader, resolver: resolver, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *ReconStep) WithClock(now func() time.Time) *ReconStep {
	s.now = now
	return s
}

// Run executes recon for batchID.
func (s *ReconStep) Run(ctx context.Context, batchID int64, actor string, progress ProgressFunc) (ReconSummary, error) {
	switch {
	case strings.TrimSpace(actor) == "":
		return ReconSummary{}, domain.ErrActorRequired
	case s.reader == nil:
		return ReconSummary{}, ErrReconReaderNotConfigured
	case s.resolver == nil:
		return ReconSummary{}, ErrCallLogNotConfigured
	}
	var sum ReconSummary
	err := s.runner.RunLocked(ctx, batchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.ReconStore)
		if !ok {
			return ErrReconStoreUnsupported
		}
		var err error
		sum, err = s.reconLocked(ctx, st, actor, progress)
		return err
	})
	if err != nil {
		return ReconSummary{}, err
	}
	return sum, nil
}

func (s *ReconStep) reconLocked(ctx context.Context, st domain.ReconStore, actor string, progress ProgressFunc) (ReconSummary, error) {
	b, err := st.GetForUpdate(ctx)
	if err != nil {
		return ReconSummary{}, err
	}
	if err := checkReconBatch(b); err != nil {
		return ReconSummary{}, err
	}
	rb, err := s.reader.ReadBackBatch(ctx, b.ID())
	if err != nil {
		return ReconSummary{}, fmt.Errorf("recon read back batch: %w", err)
	}
	adj, err := s.reader.ReadBackAdj(ctx, b.Period(), b.ID())
	if err != nil {
		return ReconSummary{}, fmt.Errorf("recon read back ADJ: %w", err)
	}
	progress.report(ctx, progressRead)
	sum := ReconSummary{
		BatchID: b.ID(), Period: b.Period(), Mode: reconModeResolveOnly,
		OracleFound: rb.Found, OracleStatus: rb.Status, RunAt: s.now(),
	}
	stdRows, err := st.ListStdRows(ctx)
	if err != nil {
		return ReconSummary{}, err
	}
	pgDigest, err := domain.ComputeStdDigest(stdRows)
	if err != nil {
		return ReconSummary{}, err
	}
	ev := newCallEvidence(b, rb, pgDigest, adj)
	if err := s.resolveCalls(ctx, b.ID(), ev, &sum); err != nil {
		return ReconSummary{}, err
	}
	progress.report(ctx, progressCompute)
	prev := b.Status()
	if prev == domain.StatusValuated || prev == domain.StatusReconciled {
		sum.Mode = reconModeFull
		if err := s.compare(ctx, st, b, rb, stdRows, pgDigest, adj, &sum); err != nil {
			return ReconSummary{}, err
		}
		if err := s.moveStatus(b, actor, &sum); err != nil {
			return ReconSummary{}, err
		}
	}
	progress.report(ctx, progressPersist)
	sum.Status = string(b.Status())
	if err := setStepSummary(b, StepRecon, sum); err != nil {
		return ReconSummary{}, err
	}
	logRecon(sum)
	return sum, st.Save(ctx, b, prev, domain.Invalidation{})
}

// checkReconBatch: LIVE (a SHADOW batch is never pushed) and pushed at least
// once (PUSHED, VALUATED, RECONCILED, or FAILED after a write attempt).
func checkReconBatch(b *domain.Batch) error {
	if b.Mode() == domain.ModeShadow {
		return fmt.Errorf("%w: batch %d", domain.ErrShadowNotPushable, b.ID())
	}
	switch b.Status() {
	case domain.StatusPushed, domain.StatusValuated, domain.StatusReconciled, domain.StatusFailed:
		return nil
	case domain.StatusDraft, domain.StatusDemandLoaded, domain.StatusCovered, domain.StatusDerived,
		domain.StatusValidated, domain.StatusLocked, domain.StatusSuperseded:
	}
	return fmt.Errorf("%w: recon in %s", ErrStepNotAllowed, b.Status())
}

// compare fills the totals / md5 / row comparison and stores the per-row
// recon columns.
func (s *ReconStep) compare(ctx context.Context, st domain.ReconStore, b *domain.Batch, rb domain.OracleBatchReadBack,
	stdRows []domain.StdRow, pgDigest domain.StdDigest, adj []domain.AdjReadBackCombo, sum *ReconSummary,
) error {
	sum.PgRowCount, sum.PgRowsMD5 = pgDigest.Totals.RowCount(), pgDigest.RowsMD5
	if rb.Found {
		oraDigest, err := domain.ComputeStdDigest(rb.CostRows)
		if err != nil {
			return fmt.Errorf("recon oracle digest: %w", err)
		}
		sum.OraRowCount, sum.OraRowsMD5 = oraDigest.Totals.RowCount(), oraDigest.RowsMD5
		sum.MD5Equal = oraDigest.Equal(pgDigest)
		t := b.Totals()
		sum.TotalsEqual = t != nil && t.Equal(rb.Header) && rb.Header.Equal(pgDigest.Totals) && rb.Period == b.Period()
		sum.RuleHashOK = rb.RuleHash == b.RuleHash()
	}
	class := domain.ClassifyRecon(stdRows, adj, b.ID())
	sum.Counts = class.Counts
	for i, k := range class.NotCovered {
		if i == reconListMax {
			break
		}
		sum.NotCovered = append(sum.NotCovered, k.String())
	}
	n, err := st.ApplyRecon(ctx, class.Rows)
	if err != nil {
		return err
	}
	sum.RowsWritten = n
	sum.Reasons = reconReasons(*sum, class)
	sum.Reconciled = len(sum.Reasons) == 0
	return nil
}

// reconReasons lists why the batch is not RECONCILED (empty: reconciled).
func reconReasons(sum ReconSummary, class domain.ReconClassification) []string {
	var out []string
	add := func(cond bool, msg string) {
		if cond {
			out = append(out, msg)
		}
	}
	add(!sum.OracleFound, "batch not found in CST_GOAPPS_STD_BATCH")
	add(sum.OracleFound && !sum.TotalsEqual, "control totals differ (PG vs Oracle header)")
	add(sum.OracleFound && !sum.MD5Equal, "rows md5 differs (PG vs Oracle cost rows)")
	add(sum.OracleFound && !sum.RuleHashOK, "rule hash differs")
	add(class.Counts.Diff > 0, fmt.Sprintf("%d DIFF row(s)", class.Counts.Diff))
	add(class.Counts.NotInAdj > 0, fmt.Sprintf("%d NOT_IN_ADJ row(s)", class.Counts.NotInAdj))
	add(class.Counts.NotCoveredItems > 0, fmt.Sprintf("%d NOT_COVERED ADJ item(s)", class.Counts.NotCoveredItems))
	add(sum.Unresolved > 0, fmt.Sprintf("%d unresolved Oracle call(s)", sum.Unresolved))
	return out
}

// moveStatus applies §5.3: VALUATED -> RECONCILED on a clean recon,
// RECONCILED -> VALUATED on a new difference, otherwise unchanged.
func (s *ReconStep) moveStatus(b *domain.Batch, actor string, sum *ReconSummary) error {
	var to domain.BatchStatus
	switch {
	case sum.Reconciled && b.Status() == domain.StatusValuated:
		to = domain.StatusReconciled
	case !sum.Reconciled && b.Status() == domain.StatusReconciled:
		to = domain.StatusValuated
	default:
		b.SetErrorText(truncateText(strings.Join(sum.Reasons, "; ")))
		return nil
	}
	if _, err := b.Transition(to, actor, s.now()); err != nil {
		return err
	}
	b.SetErrorText(truncateText(strings.Join(sum.Reasons, "; ")))
	return nil
}

func logRecon(sum ReconSummary) {
	ev := log.Info()
	if sum.Mode == reconModeFull && !sum.Reconciled {
		ev = log.Warn()
	}
	ev.Int64("batch_id", sum.BatchID).Str("period", sum.Period).Str("mode", sum.Mode).
		Int("match", sum.Counts.Match).Int("diff", sum.Counts.Diff).Int("not_in_adj", sum.Counts.NotInAdj).
		Int64("not_covered_items", sum.Counts.NotCoveredItems).Bool("totals_equal", sum.TotalsEqual).
		Bool("md5_equal", sum.MD5Equal).Int("unresolved_calls", sum.Unresolved).Bool("reconciled", sum.Reconciled).
		Str("batch_status", sum.Status).Msg("erp recon")
}
