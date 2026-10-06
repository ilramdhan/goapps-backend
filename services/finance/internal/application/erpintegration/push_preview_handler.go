package erpintegration

// push_preview_handler.go implements PreviewErpPush (plan-06 P5-T3 step 2;
// design Part 2 §8.1, §7.2): the mandatory dry run before a push. It
// re-verifies the batch exactly as the push will (status, G10, control
// totals + rows md5 against the derive digest, R-9 stale-cost check) and
// returns the totals the operator must confirm. It makes NO Oracle call.

import (
	"context"
	"errors"
	"fmt"
	"math"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Push preview / verification errors.
var (
	// ErrPushStoreUnsupported is returned when the batch runner's store does
	// not implement domain.PushStore.
	ErrPushStoreUnsupported = errors.New("erpintegration: batch store does not support the push step")
	// ErrStaleSourceCost is returned when a std row's source cost is no
	// longer the active APPROVED ACTUAL row of the period (R-9).
	ErrStaleSourceCost = errors.New("erpintegration: a source cost is no longer the active APPROVED row; re-derive")
)

// PushPreviewQuery asks for the push preview. HasPermission is the
// delivery-layer RBAC outcome for finance.cost.erpintegration.push.
type PushPreviewQuery struct {
	BatchID       int64
	HasPermission bool
}

// PushPreview is what the operator reviews and confirms before a push.
type PushPreview struct {
	BatchID       int64  `json:"batch_id"`
	Period        string `json:"period"`
	Seq           int    `json:"seq"`
	RuleHash      string `json:"rule_hash"`
	RowCount      int64  `json:"row_count"`
	SumStd        string `json:"sum_std"`
	SumConv       string `json:"sum_conv"`
	SumPvl        string `json:"sum_pvl"`
	RowsMD5       string `json:"rows_md5"`
	DigestVersion int    `json:"digest_version"`
	// PushEnabled / WriterMode report G1 / G2 for the UI; the preview itself
	// does not require them (it never reaches Oracle).
	PushEnabled bool   `json:"push_enabled"`
	WriterMode  string `json:"writer_mode"`
}

// PushPreviewHandler builds the push preview.
type PushPreviewHandler struct {
	runner      domain.BatchTxRunner
	pushEnabled bool
	writerMode  domain.WriterMode
}

// NewPushPreviewHandler builds the handler. pushEnabled and mode are only
// reported; the push step enforces them.
func NewPushPreviewHandler(runner domain.BatchTxRunner, pushEnabled bool, mode domain.WriterMode) *PushPreviewHandler {
	return &PushPreviewHandler{runner: runner, pushEnabled: pushEnabled, writerMode: mode}
}

// Handle verifies the batch under the G11 lock (read-only) and returns the
// control totals. No writer is involved.
func (h *PushPreviewHandler) Handle(ctx context.Context, q PushPreviewQuery) (PushPreview, error) {
	if err := checkPermission(q.HasPermission, ErrPushPermissionDenied); err != nil {
		return PushPreview{}, err
	}
	var out PushPreview
	err := h.runner.RunLocked(ctx, q.BatchID, func(ctx context.Context, bs domain.BatchStore) error {
		st, ok := bs.(domain.PushStore)
		if !ok {
			return ErrPushStoreUnsupported
		}
		b, err := st.GetForUpdate(ctx)
		if err != nil {
			return err
		}
		plan, err := verifyPush(ctx, st, b)
		if err != nil {
			return err
		}
		out = previewOf(b, plan.digest)
		return nil
	})
	if err != nil {
		return PushPreview{}, err
	}
	out.PushEnabled, out.WriterMode = h.pushEnabled, string(h.writerMode)
	return out, nil
}

func previewOf(b *domain.Batch, d domain.StdDigest) PushPreview {
	return PushPreview{
		BatchID: b.ID(), Period: b.Period(), Seq: b.Seq(), RuleHash: b.RuleHash(),
		RowCount: d.Totals.RowCount(), SumStd: d.Totals.SumStd().StringFixed(domain.ScaleR5),
		SumConv: d.Totals.SumConv().StringFixed(domain.ScaleR5), SumPvl: d.Totals.SumPvl().StringFixed(domain.ScaleR5),
		RowsMD5: d.RowsMD5, DigestVersion: domain.StdDigestVersion,
	}
}

// pushPlan is the verified content of a push.
type pushPlan struct {
	digest domain.StdDigest
	rows   []domain.PushRow
}

// verifyPush is the shared preview / execute verification, run inside the
// G11 transaction: batch gates, G10, digest re-verification against the
// frozen totals and the derive md5, and R-9.
func verifyPush(ctx context.Context, st domain.PushStore, b *domain.Batch) (pushPlan, error) {
	if err := checkPushBatch(b); err != nil {
		return pushPlan{}, err
	}
	if err := checkPeriodLocked(ctx, st, b.Period()); err != nil {
		return pushPlan{}, err
	}
	rows, err := st.ListStdRows(ctx)
	if err != nil {
		return pushPlan{}, err
	}
	d, err := domain.ComputeStdDigest(rows)
	if err != nil {
		return pushPlan{}, err
	}
	if err := checkDigest(b, d); err != nil {
		return pushPlan{}, err
	}
	if err := checkSourceCosts(ctx, st, b.Period(), rows); err != nil {
		return pushPlan{}, err
	}
	pr, err := pushRows(rows)
	if err != nil {
		return pushPlan{}, err
	}
	return pushPlan{digest: d, rows: pr}, nil
}

// checkDigest compares the recomputed digest with the batch control totals
// and the derive summary's rows md5 (the rows changed since derive: stale).
func checkDigest(b *domain.Batch, d domain.StdDigest) error {
	t := b.Totals()
	if t == nil {
		return domain.ErrControlTotalsMissing
	}
	var ds DeriveSummary
	if !readSummaryEntry(b.Summary(), StepDerive, &ds) || ds.RowsMD5 == "" {
		return fmt.Errorf("%w: derive digest missing", domain.ErrPreviewStale)
	}
	if ds.DigestVersion != domain.StdDigestVersion {
		return fmt.Errorf("%w: digest version %d", domain.ErrPreviewStale, ds.DigestVersion)
	}
	if !d.Totals.Equal(*t) || d.RowsMD5 != ds.RowsMD5 {
		return fmt.Errorf("%w: std rows differ from the derived control totals", domain.ErrPreviewStale)
	}
	if d.Totals.RowCount() == 0 {
		return fmt.Errorf("%w: no OK rows to push", domain.ErrValidationFailed)
	}
	return nil
}

// checkSourceCosts is R-9: every source cost of an OK row must still be the
// active APPROVED ACTUAL row of the period at the same version.
func checkSourceCosts(ctx context.Context, st domain.PushStore, period string, rows []domain.StdRow) error {
	want := map[int64]*int32{}
	ids := make([]int64, 0)
	for _, r := range rows {
		if r.Status != domain.DeriveOK || r.AxCostSysID == nil {
			continue
		}
		if _, seen := want[*r.AxCostSysID]; !seen {
			ids = append(ids, *r.AxCostSysID)
		}
		want[*r.AxCostSysID] = r.AxCostVersion
	}
	if len(ids) == 0 {
		return nil
	}
	got, err := st.LoadAxComponents(ctx, period, ids)
	if err != nil {
		return fmt.Errorf("check source costs: %w", err)
	}
	for _, id := range ids {
		a, ok := got[id]
		if !ok {
			return fmt.Errorf("%w: cost %d", ErrStaleSourceCost, id)
		}
		if v := want[id]; v != nil && *v != a.Version {
			return fmt.Errorf("%w: cost %d version %d != %d", ErrStaleSourceCost, id, *v, a.Version)
		}
	}
	return nil
}

// pushRows maps the OK std rows to W1 cost rows (non-OK rows never go to
// Oracle).
func pushRows(rows []domain.StdRow) ([]domain.PushRow, error) {
	out := make([]domain.PushRow, 0, len(rows))
	for _, r := range rows {
		if r.Status != domain.DeriveOK {
			continue
		}
		if !r.StdCost.Valid {
			return nil, fmt.Errorf("%w: %s/%s/%s has no std cost", domain.ErrValidationFailed,
				r.Key.ItemCode, r.Key.GradeCode, r.Key.ShadeCode)
		}
		out = append(out, domain.PushRow{
			ItemCode: r.Key.ItemCode, GradeCode: r.Key.GradeCode, ShadeCode: r.Key.ShadeCode,
			ItemName: r.ItemName, ShadeName: r.ShadeName, Source: string(r.Source),
			StdCost: r.StdCost.Decimal, ConvCost: r.ConvCost,
			ConvCost1: r.ConvCost1, ConvCost2: r.ConvCost2, ConvCost4: r.ConvCost4, ConvCost5: r.ConvCost5,
			ChpConKg: r.ChpConKg, ChpCost: r.ChpCost, ChpItemCode: r.ChpItemCode,
			FgType: r.FgType, Basis: string(r.Basis), SellingPrice: r.SellingPrice,
			AxCost: r.AxCost, AxConvCost: r.AxConvCost, ValueLoss: r.ValueLoss, ProdValLoss: r.ProdValLoss,
			MsBatchItem: r.MsBatchItem, ItemType: r.ItemType, PrdPerDay: r.PrdPerDay,
			AxCostSysID: r.AxCostSysID, AxCostVersion: r.AxCostVersion,
		})
	}
	return out, nil
}

// seqInt32 narrows the batch seq for PushBatch (gosec G115).
func seqInt32(seq int) (int32, error) {
	if seq < 1 || seq > math.MaxInt32 {
		return 0, fmt.Errorf("%w: batch seq %d out of range", ErrInvalidJobParams, seq)
	}
	return int32(seq), nil
}
