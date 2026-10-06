package erpintegration

// valuation_preview_handler.go implements the snapshot-first valuation
// preview (plan-06 P5-T4; design Part 1 §3.2 G1-G7a, §3.5, §4.7, §9.4).
//
// Oracle is only READ here: the ADJ rows come from domain.AdjSnapshotReader
// (SELECT through the read-only querier, no FOR UPDATE). Nothing in this
// handler calls the Oracle writer. The preview and the immutable PG
// snapshot are committed in one PG transaction (G7) before any later
// execute step can reference them; the G7a V-07 period-set pre-validation
// runs over the whole eligible set and one offending head refuses the whole
// preview (User decision U-1: the package is period-wide).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration/validation"
)

// Valuation preview errors.
var (
	// ErrValuatePermissionDenied is returned when the caller lacks the
	// .valuate permission (G3).
	ErrValuatePermissionDenied = errors.New("erpintegration: permission denied to valuate")
	// ErrAdjSnapshotReaderNotConfigured is returned when no read-only ADJ
	// snapshot reader is wired (Oracle absent): the preview fails closed.
	ErrAdjSnapshotReaderNotConfigured = errors.New("erpintegration: ADJ snapshot reader not configured")
	// ErrNoEligibleAdj is returned when the period has no unposted,
	// unapproved guarded ADJ item: no preview is created.
	ErrNoEligibleAdj = errors.New("erpintegration: no eligible unposted ADJ rows for the period")
	// ErrPreviewNotConfigured is returned when a required preview port
	// (batch repository, std rows or preview repository) is not wired.
	ErrPreviewNotConfigured = errors.New("erpintegration: valuation preview not configured")
)

// defaultPreviewTTL is used when the configured TTL is not positive.
const defaultPreviewTTL = 30 * time.Minute

// previewFindingSample bounds the V-07 findings stored on the preview.
const previewFindingSample = 50

// StdRowLister lists the std rows of a batch (postgres.ErpStdCostRepository).
type StdRowLister interface {
	List(ctx context.Context, batchID int64) ([]domain.StdRow, error)
}

// ValuationPreviewConfig holds the G1/G2 flags and the preview TTL.
type ValuationPreviewConfig struct {
	// ValuationEnabled is G1 (erp.valuation_enabled, default false).
	ValuationEnabled bool
	// WriterMode is the effective writer mode (G2: must not be disabled).
	WriterMode domain.WriterMode
	// TTL is the preview lifetime (erp.preview_ttl, default 30m).
	TTL time.Duration
}

// ValuationPreviewCommand requests an ADJ preview for a batch.
type ValuationPreviewCommand struct {
	BatchID int64
	Actor   string
	// Operation is VALUATE, APPROVE or RESTORE; empty means VALUATE.
	Operation domain.AdjOperation
	// HasPermission is the delivery-layer result of the RBAC check for the
	// operation (.valuate / .approve / .restore).
	HasPermission bool
}

func (c ValuationPreviewCommand) op() domain.AdjOperation {
	if c.Operation == "" {
		return domain.AdjOpValuate
	}
	return c.Operation
}

// PreviewHead is one eligible head of the preview.
type PreviewHead struct {
	HeadSysID      int64           `json:"head_sys_id"`
	TxnCode        string          `json:"txn_code"`
	Items          int             `json:"items"`
	ProjectedItems int             `json:"projected_items"`
	CurrentVal     decimal.Decimal `json:"current_val"`
	ProjectedVal   decimal.Decimal `json:"projected_val"`
}

// PreviewV07 is the stored G7a result.
type PreviewV07 struct {
	OK               bool     `json:"ok"`
	HeadCount        int      `json:"head_count"`
	ItemCount        int      `json:"item_count"`
	OffendingHeadIDs []int64  `json:"offending_head_ids"`
	Findings         []string `json:"findings,omitempty"`
}

// PreviewTotals is the cevp_totals JSON.
type PreviewTotals struct {
	Period         string          `json:"period"`
	BatchID        int64           `json:"batch_id"`
	HeadCount      int             `json:"head_count"`
	ItemCount      int             `json:"item_count"`
	ProjectedItems int             `json:"projected_items"`
	CurrentVal     decimal.Decimal `json:"current_val"`
	ProjectedVal   decimal.Decimal `json:"projected_val"`
	Heads          []PreviewHead   `json:"heads"`
	V07            PreviewV07      `json:"v07"`
}

// ValuationPreviewResult is returned to the caller.
type ValuationPreviewResult struct {
	Preview     domain.ValuationPreview
	Totals      PreviewTotals
	Excluded    domain.AdjExclusions
	ConfirmText string
}

// ValuationPreviewHandler builds VALUATE previews.
type ValuationPreviewHandler struct {
	batches  domain.BatchRepository
	locks    PeriodLockChecker
	reader   domain.AdjSnapshotReader
	std      StdRowLister
	previews domain.ValuationPreviewRepository
	cfg      ValuationPreviewConfig
	now      func() time.Time
}

// NewValuationPreviewHandler builds the handler. reader nil (no Oracle)
// makes every preview fail closed with ErrAdjSnapshotReaderNotConfigured.
func NewValuationPreviewHandler(batches domain.BatchRepository, locks PeriodLockChecker, reader domain.AdjSnapshotReader,
	std StdRowLister, previews domain.ValuationPreviewRepository, cfg ValuationPreviewConfig,
) *ValuationPreviewHandler {
	if cfg.TTL <= 0 {
		cfg.TTL = defaultPreviewTTL
	}
	return &ValuationPreviewHandler{batches: batches, locks: locks, reader: reader, std: std, previews: previews, cfg: cfg, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *ValuationPreviewHandler) WithClock(now func() time.Time) *ValuationPreviewHandler {
	h.now = now
	return h
}

// Handle runs the gates, snapshots the eligible ADJ rows, pre-validates the
// set and stores an OPEN preview (or a FAILED one on a V-07 refusal).
func (h *ValuationPreviewHandler) Handle(ctx context.Context, cmd ValuationPreviewCommand) (ValuationPreviewResult, error) {
	if err := h.checkStatic(cmd); err != nil {
		return ValuationPreviewResult{}, err
	}
	b, err := h.batches.GetByID(ctx, cmd.BatchID)
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview: %w", err)
	}
	if err := h.checkBatch(ctx, cmd.op(), b); err != nil {
		return ValuationPreviewResult{}, err
	}
	rows, err := h.reader.SnapshotAdjRows(ctx, b.Period())
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview: %w", err)
	}
	eligible, excluded := domain.SplitAdjSnapshot(rows)
	if cmd.op() != domain.AdjOpValuate {
		return h.handleStamped(ctx, cmd, b, rows, eligible, excluded)
	}
	if len(eligible) == 0 {
		return ValuationPreviewResult{}, fmt.Errorf("%w: period %s (%d rows read)", ErrNoEligibleAdj, b.Period(), len(rows))
	}
	stdRows, err := h.std.List(ctx, b.ID())
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview std rows: %w", err)
	}
	projected, unmatched, err := projectSnapshot(eligible, stdRows, b.ID())
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview: %w", err)
	}
	excluded.UnmatchedItems = unmatched
	v07 := validation.PrevalidatePeriodSet(b.Period(), validation.ProjectRates(adjSetHeads(projected), stdRows))
	totals := buildPreviewTotals(b, projected, v07)
	return h.store(ctx, cmd, b, projected, excluded, totals, v07)
}

// handleStamped previews APPROVE / RESTORE over the rows stamped with the
// batch id, using the same selection as the execute specs so SetHash matches.
func (h *ValuationPreviewHandler) handleStamped(ctx context.Context, cmd ValuationPreviewCommand, b *domain.Batch,
	rows, eligible []domain.AdjSnapshotRow, excluded domain.AdjExclusions,
) (ValuationPreviewResult, error) {
	if excluded.PostedHeads > 0 {
		return ValuationPreviewResult{}, fmt.Errorf("%w: %d posted head(s) in %s", domain.ErrPeriodPosted, excluded.PostedHeads, b.Period())
	}
	stdRows, err := h.std.List(ctx, b.ID())
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview std rows: %w", err)
	}
	spec, err := adjSpecFor(cmd.op())
	if err != nil {
		return ValuationPreviewResult{}, err
	}
	set, err := spec.selectSet(rows, eligible, stdRows, b.ID())
	if err != nil {
		return ValuationPreviewResult{}, err
	}
	if len(set.rows) == 0 {
		return ValuationPreviewResult{}, fmt.Errorf("%w: period %s (%d rows read)", ErrNoEligibleAdj, b.Period(), len(rows))
	}
	v07 := validation.PeriodSetResult{}
	if spec.validate {
		v07 = validation.PrevalidatePeriodSet(b.Period(), validation.ProjectRates(adjSetHeads(set.rows), stdRows))
	}
	totals := buildPreviewTotals(b, set.rows, v07)
	return h.store(ctx, cmd, b, set.rows, excluded, totals, v07)
}

// checkStatic runs the gates that need no I/O: G3, G1, G2 and wiring.
func (h *ValuationPreviewHandler) checkStatic(cmd ValuationPreviewCommand) error {
	switch {
	case !cmd.HasPermission:
		return ErrValuatePermissionDenied
	case cmd.Actor == "":
		return domain.ErrActorRequired
	case !h.cfg.ValuationEnabled:
		return fmt.Errorf("%w: erp.valuation_enabled is off", domain.ErrFeatureDisabled)
	case h.cfg.WriterMode == "" || h.cfg.WriterMode == domain.WriterModeDisabled:
		return fmt.Errorf("%w: writer mode is disabled", domain.ErrWriterNotConfigured)
	case h.reader == nil:
		return ErrAdjSnapshotReaderNotConfigured
	case h.locks == nil:
		return ErrPeriodLockNotConfigured
	case h.batches == nil || h.std == nil || h.previews == nil:
		return ErrPreviewNotConfigured
	}
	return nil
}

// checkBatch requires a LIVE batch in the status the operation's execute
// spec accepts; VALUATE also needs a locked period (G10).
func (h *ValuationPreviewHandler) checkBatch(ctx context.Context, op domain.AdjOperation, b *domain.Batch) error {
	if b.Mode() == domain.ModeShadow {
		return fmt.Errorf("%w: batch %d", domain.ErrShadowNotPushable, b.ID())
	}
	switch op {
	case domain.AdjOpValuate:
	case domain.AdjOpApprove:
		if err := requireStatus(b, "APPROVE_ADJ", domain.StatusReconciled); err != nil {
			return err
		}
		if b.AdjApproved() != nil {
			return fmt.Errorf("%w: batch %d ADJ already approved", ErrStepNotAllowed, b.ID())
		}
		return nil
	case domain.AdjOpRestore:
		if err := requireStatus(b, "RESTORE_ADJ", domain.StatusValuated, domain.StatusReconciled); err != nil {
			return err
		}
		if b.AdjApproved() != nil {
			return fmt.Errorf("%w: batch %d ADJ already approved", ErrAdjIneligible, b.ID())
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrAdjOperationUnknown, op)
	}
	if b.Status() != domain.StatusPushed {
		return fmt.Errorf("%w: valuation preview needs PUSHED, batch %d is %s", ErrStepNotAllowed, b.ID(), b.Status())
	}
	locked, err := h.locks.IsLocked(ctx, b.Period(), periodLockCalcType)
	if err != nil {
		return fmt.Errorf("valuation preview period lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("%w: period %s", domain.ErrPeriodNotLocked, b.Period())
	}
	return nil
}

// store persists the preview: FAILED (and an error) on a V-07 refusal,
// otherwise OPEN with its snapshot.
func (h *ValuationPreviewHandler) store(ctx context.Context, cmd ValuationPreviewCommand, b *domain.Batch,
	rows []domain.AdjSnapshotRow, excluded domain.AdjExclusions, totals PreviewTotals, v07 validation.PeriodSetResult,
) (ValuationPreviewResult, error) {
	raw, err := json.Marshal(totals)
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview totals: %w", err)
	}
	now := h.now().UTC()
	p := domain.ValuationPreview{
		BatchID: b.ID(), Operation: cmd.op(),
		HeadCount: totals.HeadCount, ItemCount: totals.ItemCount,
		Excluded: excluded, SetHash: domain.AdjSetHash(rows), Totals: raw,
		ConfirmText: domain.PreviewConfirmText(b.Period(), b.ID()),
		CreatedBy:   cmd.Actor, CreatedAt: now, ExpiresAt: now.Add(h.cfg.TTL),
	}
	res := ValuationPreviewResult{Totals: totals, Excluded: excluded, ConfirmText: p.ConfirmText}
	if !v07.OK() {
		refusal := fmt.Errorf("%w: V-07 period-set pre-validation refused %d head(s): %v",
			domain.ErrValidationFailed, len(v07.Offending), v07.OffendingHeadIDs())
		failed, ferr := h.previews.CreateFailed(ctx, p)
		if ferr != nil {
			return res, errors.Join(refusal, fmt.Errorf("record failed preview: %w", ferr))
		}
		res.Preview = failed
		return res, refusal
	}
	stored, err := h.previews.CreateOpen(ctx, p, b.Period(), b.Status(), rows)
	if err != nil {
		return ValuationPreviewResult{}, fmt.Errorf("valuation preview store: %w", err)
	}
	res.Preview = stored
	return res, nil
}

// projectSnapshot fills the projected values of every eligible row and
// counts the rows without a valued std row.
func projectSnapshot(rows []domain.AdjSnapshotRow, stdRows []domain.StdRow, batchID int64) ([]domain.AdjSnapshotRow, int, error) {
	byKey := domain.StdRowsByKey(stdRows)
	out := make([]domain.AdjSnapshotRow, len(rows))
	unmatched := 0
	for i, r := range rows {
		var std *domain.StdRow
		if s, ok := byKey[r.Key()]; ok {
			std = &s
		}
		p, err := domain.ProjectAdjRow(r, std, batchID)
		if err != nil {
			return nil, 0, err
		}
		if !p.IsProjected() {
			unmatched++
		}
		out[i] = p
	}
	return out, unmatched, nil
}

// adjSetHeads groups the rows by head for the V-07 period-set check.
func adjSetHeads(rows []domain.AdjSnapshotRow) []validation.AdjSetHead {
	idx := map[int64]int{}
	var heads []validation.AdjSetHead
	for _, r := range rows {
		i, ok := idx[r.HeadSysID]
		if !ok {
			i = len(heads)
			idx[r.HeadSysID] = i
			heads = append(heads, validation.AdjSetHead{HeadSysID: r.HeadSysID, TxnCode: r.TxnCode})
		}
		heads[i].Items = append(heads[i].Items, validation.AdjSetItem{
			ItemSysID: r.ItemSysID, ItemCode: r.ItemCode, GradeCode: r.GradeCode, ShadeCode: r.ShadeCode,
			CurrentRate: r.Rate,
		})
	}
	sort.Slice(heads, func(a, c int) bool { return heads[a].HeadSysID < heads[c].HeadSysID })
	return heads
}

// buildPreviewTotals computes Σ current / projected ADJI_VAL per head and in
// total (an unprojected row keeps its current value) and the V-07 record.
func buildPreviewTotals(b *domain.Batch, rows []domain.AdjSnapshotRow, v07 validation.PeriodSetResult) PreviewTotals {
	t := PreviewTotals{Period: b.Period(), BatchID: b.ID(), ItemCount: len(rows)}
	idx := map[int64]int{}
	for _, r := range rows {
		i, ok := idx[r.HeadSysID]
		if !ok {
			i = len(t.Heads)
			idx[r.HeadSysID] = i
			t.Heads = append(t.Heads, PreviewHead{HeadSysID: r.HeadSysID, TxnCode: r.TxnCode})
		}
		cur, proj := valOrZero(r.Val), valOrZero(r.Val)
		if r.IsProjected() {
			proj = valOrZero(r.NewVal)
			t.Heads[i].ProjectedItems++
			t.ProjectedItems++
		}
		t.Heads[i].Items++
		t.Heads[i].CurrentVal = t.Heads[i].CurrentVal.Add(cur)
		t.Heads[i].ProjectedVal = t.Heads[i].ProjectedVal.Add(proj)
		t.CurrentVal = t.CurrentVal.Add(cur)
		t.ProjectedVal = t.ProjectedVal.Add(proj)
	}
	sort.Slice(t.Heads, func(a, c int) bool { return t.Heads[a].HeadSysID < t.Heads[c].HeadSysID })
	t.HeadCount = len(t.Heads)
	t.V07 = PreviewV07{OK: v07.OK(), HeadCount: v07.HeadCount, ItemCount: v07.ItemCount, OffendingHeadIDs: v07.OffendingHeadIDs()}
	for i, f := range v07.Findings() {
		if i >= previewFindingSample {
			break
		}
		t.V07.Findings = append(t.V07.Findings, f.Message)
	}
	return t
}

func valOrZero(d decimal.NullDecimal) decimal.Decimal {
	if !d.Valid {
		return decimal.Zero
	}
	return d.Decimal
}
