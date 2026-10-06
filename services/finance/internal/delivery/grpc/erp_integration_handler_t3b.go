package grpc

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	periodlockapp "github.com/mutugading/goapps-backend/services/finance/internal/application/periodlock"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	domainperiodlock "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// T3b collaborator seams.
type (
	erpAbandoner interface {
		Handle(ctx context.Context, cmd erpapp.AbandonBatchCommand) (*domain.Batch, error)
	}
	erpPreviewGetter interface {
		Get(ctx context.Context, id string) (domain.ValuationPreview, error)
	}
	erpScheduler interface {
		Get(ctx context.Context, hasPermission bool) (erpapp.ScheduleView, error)
		Update(ctx context.Context, cmd erpapp.UpdateScheduleCommand) (erpapp.ScheduleView, error)
	}
	erpPeriodLockGetter interface {
		Handle(ctx context.Context, q periodlockapp.GetQuery) (*domainperiodlock.PeriodLock, error)
	}
	erpCoverageLister interface {
		List(ctx context.Context, batchID int64, statuses ...domain.CoverageStatus) ([]domain.CoverageLine, error)
	}
	erpStdCostLister interface {
		List(ctx context.Context, batchID int64) ([]domain.StdRow, error)
	}
	erpOracleCallLister interface {
		ListByBatch(ctx context.Context, batchID int64) ([]domain.OracleCallRow, error)
	}
	erpSanityReporter interface {
		Report(ctx context.Context, f erpapp.CurrencySanityFilter) (*erpapp.CurrencySanityReport, error)
	}
	erpLinkReporter interface {
		Handle(ctx context.Context, q erpapp.LinkReadinessQuery) (*erpapp.LinkReadinessReport, error)
	}
	erpBacktestGetter interface {
		Get(ctx context.Context, batchID int64) (domain.BacktestReport, error)
	}
)

// ErpConfigView is the read-only, secret-free config exposed to operators.
type ErpConfigView struct {
	PushEnabled, ValuationEnabled, AdjApproveEnabled bool
	WriterMode                                       string
	OracleIFConfigured                               bool
}

func erpNullDec(d decimal.NullDecimal) string {
	if !d.Valid {
		return ""
	}
	return d.Decimal.String()
}

func erpEnum(m map[string]int32, prefix, v string) int32 { return m[prefix+v] }

func erpPage(p *commonv1.PaginationRequest, total int) (lo, hi int, resp *commonv1.PaginationResponse) {
	page, size := int32(1), int32(20)
	if p != nil {
		if p.GetPage() > 0 {
			page = p.GetPage()
		}
		if p.GetPageSize() > 0 {
			size = p.GetPageSize()
		}
	}
	t := int64(total)
	l := min(int64(page-1)*int64(size), t)
	h := min(l+int64(size), t)
	pages := int32((t + int64(size) - 1) / int64(size)) //nolint:gosec // bounded by page size
	return int(l), int(h), &commonv1.PaginationResponse{CurrentPage: page, PageSize: size, TotalItems: t, TotalPages: pages}
}

func erpAdjPermFor(op domain.AdjOperation) string {
	switch op {
	case domain.AdjOpApprove:
		return permErpApprove
	case domain.AdjOpRestore:
		return permErpRestore
	case domain.AdjOpValuate:
		return permErpValuate
	}
	return permErpValuate
}

// GetErpAdjPreview returns a stored ADJ preview.
func (h *ErpIntegrationHandler) GetErpAdjPreview(ctx context.Context, req *financev1.GetErpAdjPreviewRequest) (*financev1.GetErpAdjPreviewResponse, error) {
	p, err := h.d.Previews.Get(ctx, req.GetPreviewId())
	if err != nil {
		return &financev1.GetErpAdjPreviewResponse{Base: erpErrBase(err)}, nil
	}
	var tot erpapp.PreviewTotals
	if err := jsonUnmarshalLenient(p.Totals, &tot); err != nil {
		return &financev1.GetErpAdjPreviewResponse{Base: erpErrBase(err)}, nil
	}
	heads := make([]*financev1.ErpAdjHead, 0, len(tot.Heads))
	for _, hd := range tot.Heads {
		heads = append(heads, &financev1.ErpAdjHead{
			HeadId: itoa(hd.HeadSysID), ItemCode: hd.TxnCode, Qty: itoa(int64(hd.Items)), Amount: hd.ProjectedVal.String(),
		})
	}
	lo, hi, pg := erpPage(req.GetPagination(), len(heads))
	return &financev1.GetErpAdjPreviewResponse{
		Base: erpOK("OK"), Pagination: pg,
		Data: &financev1.ErpAdjPreview{
			PreviewId: p.ID, BatchId: p.BatchID,
			Operation: financev1.ErpAdjOperation(erpEnum(financev1.ErpAdjOperation_value, "ERP_ADJ_OPERATION_", string(p.Operation))),
			SetHash:   p.SetHash, HeadCount: int64(p.HeadCount),
			ExcludedCount: int64(p.Excluded.PostedHeads + p.Excluded.ApprovedHeads),
			TotalAmount:   tot.ProjectedVal.String(), ConfirmText: p.ConfirmText,
			CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339), Heads: heads[lo:hi],
			ExpiresAt: p.ExpiresAt.UTC().Format(time.RFC3339),
		},
	}, nil
}

// ExecuteErpAdjOperation enqueues the confirmed ADJ operation.
func (h *ErpIntegrationHandler) ExecuteErpAdjOperation(ctx context.Context, req *financev1.ExecuteErpAdjOperationRequest) (*financev1.ExecuteErpAdjOperationResponse, error) {
	p, err := h.d.Previews.Get(ctx, req.GetPreviewId())
	if err != nil {
		return &financev1.ExecuteErpAdjOperationResponse{Base: erpErrBase(err)}, nil
	}
	ex, err := h.d.Steps.TriggerAdjExecute(ctx, erpapp.AdjTriggerCommand{
		BatchID: p.BatchID, PreviewID: p.ID, Operation: p.Operation,
		ConfirmSetHash: req.GetConfirmSetHash(), ConfirmText: req.GetConfirmText(),
		Actor: erpActor(ctx), HasPermission: HasPermission(ctx, erpAdjPermFor(p.Operation)),
	})
	if err != nil {
		return &financev1.ExecuteErpAdjOperationResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.ExecuteErpAdjOperationResponse{Base: erpOK("Operation queued"), JobId: ex.ID().String(), BatchId: p.BatchID}, nil
}

var erpScheduleModes = map[domain.ScheduleMode]financev1.ErpScheduleMode{
	domain.ScheduleEndOfMonth:   financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_END_OF_MONTH,
	domain.ScheduleStartOfMonth: financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_START_OF_MONTH,
	domain.ScheduleDayOfMonth:   financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_DAY_OF_MONTH,
	domain.ScheduleSpecificDate: financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_SPECIFIC_DATE,
	domain.ScheduleCron:         financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_CRON,
}

func erpScheduleModeToProto(m domain.ScheduleMode) financev1.ErpScheduleMode {
	return erpScheduleModes[m] // zero value = UNSPECIFIED
}

func erpScheduleModeFromProto(m financev1.ErpScheduleMode) domain.ScheduleMode {
	for k, v := range erpScheduleModes {
		if v == m {
			return k
		}
	}
	return ""
}

func erpScheduleToProto(v erpapp.ScheduleView) *financev1.ErpSchedule {
	out := &financev1.ErpSchedule{Enabled: v.Effective.Enabled, Source: v.Effective.Source, ValidationErrors: v.Effective.Errors}
	if s := v.Setting; s != nil {
		out.Cron, out.RunDayOfMonth, out.RunTime = s.Cron, int32(s.DayOfMonth), s.RunTime //nolint:gosec // 0..31
		out.Mode, out.RunDate, out.Timezone = erpScheduleModeToProto(s.Mode), s.RunDate, s.Timezone
	} else {
		out.Mode, out.Timezone = erpScheduleModeToProto(v.Effective.Mode), v.Effective.Timezone
	}
	if v.NextRun != nil {
		out.NextRunAt = v.NextRun.UTC().Format(time.RFC3339)
	}
	return out
}

// GetErpIntegrationSchedule returns the effective schedule.
func (h *ErpIntegrationHandler) GetErpIntegrationSchedule(ctx context.Context, _ *financev1.GetErpIntegrationScheduleRequest) (*financev1.GetErpIntegrationScheduleResponse, error) {
	v, err := h.d.Schedule.Get(ctx, true) // interceptor enforced .view
	if err != nil {
		return &financev1.GetErpIntegrationScheduleResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.GetErpIntegrationScheduleResponse{Base: erpOK("OK"), Data: erpScheduleToProto(v)}, nil
}

// UpdateErpIntegrationSchedule overlays the request on the stored setting.
func (h *ErpIntegrationHandler) UpdateErpIntegrationSchedule(ctx context.Context, req *financev1.UpdateErpIntegrationScheduleRequest) (*financev1.UpdateErpIntegrationScheduleResponse, error) {
	cur, err := h.d.Schedule.Get(ctx, true)
	if err != nil {
		return &financev1.UpdateErpIntegrationScheduleResponse{Base: erpErrBase(err)}, nil
	}
	var s domain.ScheduleSetting
	if cur.Setting != nil {
		s = *cur.Setting
	}
	s.Enabled = req.GetEnabled()
	s.Mode = erpScheduleModeFromProto(req.GetMode())
	s.DayOfMonth, s.RunTime = int(req.GetRunDayOfMonth()), req.GetRunTime()
	s.RunDate, s.Cron, s.Timezone = req.GetRunDate(), req.GetCron(), req.GetTimezone()
	v, err := h.d.Schedule.Update(ctx, erpapp.UpdateScheduleCommand{
		Setting: s, Actor: erpActor(ctx), HasPermission: HasPermission(ctx, "finance.cost.erpintegration.update"),
	})
	if err != nil {
		return &financev1.UpdateErpIntegrationScheduleResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.UpdateErpIntegrationScheduleResponse{Base: erpOK("Schedule updated"), Data: erpScheduleToProto(v)}, nil
}

// GetErpIntegrationConfig exposes flags only (never secrets).
func (h *ErpIntegrationHandler) GetErpIntegrationConfig(_ context.Context, _ *financev1.GetErpIntegrationConfigRequest) (*financev1.GetErpIntegrationConfigResponse, error) {
	c := h.d.Config
	return &financev1.GetErpIntegrationConfigResponse{Base: erpOK("OK"), Data: &financev1.ErpIntegrationConfig{
		PushEnabled: c.PushEnabled, ValuationEnabled: c.ValuationEnabled, AdjApproveEnabled: c.AdjApproveEnabled,
		WriterMode: c.WriterMode, OracleIfConfigured: c.OracleIFConfigured,
	}}, nil
}

// GetErpPeriodLock returns the lock state of a period.
func (h *ErpIntegrationHandler) GetErpPeriodLock(ctx context.Context, req *financev1.GetErpPeriodLockRequest) (*financev1.GetErpPeriodLockResponse, error) {
	l, err := h.d.PeriodLock.Handle(ctx, periodlockapp.GetQuery{Period: req.GetPeriod()})
	if err != nil {
		return &financev1.GetErpPeriodLockResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.GetErpPeriodLockResponse{Base: erpOK("OK"), Data: erpLockToProto(l)}, nil
}

// ListErpCoverage lists the batch coverage lines.
func (h *ErpIntegrationHandler) ListErpCoverage(ctx context.Context, req *financev1.ListErpCoverageRequest) (*financev1.ListErpCoverageResponse, error) {
	var sts []domain.CoverageStatus
	if st := req.GetStatus(); st != financev1.ErpCoverageStatus_ERP_COVERAGE_STATUS_UNSPECIFIED {
		sts = append(sts, domain.CoverageStatus(trimPrefix(st.String(), "ERP_COVERAGE_STATUS_")))
	}
	lines, err := h.d.Coverage.List(ctx, req.GetBatchId(), sts...)
	if err != nil {
		return &financev1.ListErpCoverageResponse{Base: erpErrBase(err)}, nil
	}
	rows := make([]*financev1.ErpCoverageLine, 0, len(lines))
	for _, l := range lines {
		if q := req.GetSearch(); q != "" && !containsFold(l.ItemCode, q) {
			continue
		}
		r := &financev1.ErpCoverageLine{
			ErpItemCode: l.ItemCode, Message: l.Reason,
			Status: financev1.ErpCoverageStatus(erpEnum(financev1.ErpCoverageStatus_value, "ERP_COVERAGE_STATUS_", string(l.Status))),
		}
		if l.ProductSysID != nil {
			r.ProductId = itoa(*l.ProductSysID)
		}
		r.CecId = l.ID
		rows = append(rows, r)
	}
	lo, hi, pg := erpPage(req.GetPagination(), len(rows))
	return &financev1.ListErpCoverageResponse{Base: erpOK("OK"), Data: rows[lo:hi], Pagination: pg}, nil
}

// ListErpStdCost lists the derived standard-cost rows.
func (h *ErpIntegrationHandler) ListErpStdCost(ctx context.Context, req *financev1.ListErpStdCostRequest) (*financev1.ListErpStdCostResponse, error) {
	all, err := h.d.StdCost.List(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ListErpStdCostResponse{Base: erpErrBase(err)}, nil
	}
	rows := make([]*financev1.ErpStdCostRow, 0, len(all))
	for _, r := range all {
		src := financev1.ErpStdSource(erpEnum(financev1.ErpStdSource_value, "ERP_STD_SOURCE_", string(r.Source)))
		if req.GetSource() != financev1.ErpStdSource_ERP_STD_SOURCE_UNSPECIFIED && src != req.GetSource() {
			continue
		}
		if b := req.GetBasis(); b != "" && string(r.Basis) != b {
			continue
		}
		if s := req.GetStatus(); s != "" && string(r.Status) != s {
			continue
		}
		rows = append(rows, &financev1.ErpStdCostRow{
			ErpItemCode: r.Key.ItemCode, GradeCode: r.Key.GradeCode, ShadeCode: r.Key.ShadeCode,
			Basis: string(r.Basis), Source: src,
			DeriveStatus: financev1.ErpDeriveStatus(erpEnum(financev1.ErpDeriveStatus_value, "ERP_DERIVE_STATUS_", string(r.Status))),
			StdCost:      erpNullDec(r.StdCost),
		})
	}
	lo, hi, pg := erpPage(req.GetPagination(), len(rows))
	return &financev1.ListErpStdCostResponse{Base: erpOK("OK"), Data: rows[lo:hi], Pagination: pg}, nil
}

// ListErpOracleCalls lists the batch's Oracle call log.
func (h *ErpIntegrationHandler) ListErpOracleCalls(ctx context.Context, req *financev1.ListErpOracleCallsRequest) (*financev1.ListErpOracleCallsResponse, error) {
	all, err := h.d.OracleCall.ListByBatch(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ListErpOracleCallsResponse{Base: erpErrBase(err)}, nil
	}
	lo, hi, pg := erpPage(req.GetPagination(), len(all))
	rows := make([]*financev1.ErpOracleCall, 0, hi-lo)
	for _, c := range all[lo:hi] {
		finished := ""
		if c.FinishedAt != nil {
			finished = c.FinishedAt.UTC().Format(time.RFC3339)
		}
		rows = append(rows, &financev1.ErpOracleCall{
			Id: c.ID, CallType: c.StatementKey, Status: c.Status, Actor: c.Actor,
			StartedAt: c.StartedAt.UTC().Format(time.RFC3339), DurationMs: c.DurationMs, Error: c.Error,
			OraCode: c.OraCode, Attempts: c.Attempts, FinishedAt: finished,
		})
	}
	return &financev1.ListErpOracleCallsResponse{Base: erpOK("OK"), Data: rows, Pagination: pg}, nil
}

// GetErpCurrencySanity reports label distribution and outliers per period.
func (h *ErpIntegrationHandler) GetErpCurrencySanity(ctx context.Context, req *financev1.GetErpCurrencySanityRequest) (*financev1.GetErpCurrencySanityResponse, error) {
	rep, err := h.d.Sanity.Report(ctx, erpapp.CurrencySanityFilter{Period: req.GetPeriod()})
	if err != nil {
		return &financev1.GetErpCurrencySanityResponse{Base: erpErrBase(err)}, nil
	}
	rows := make([]*financev1.ErpCurrencySanityRow, 0, len(rep.Labels)+len(rep.Outliers))
	for _, l := range rep.Labels {
		rows = append(rows, &financev1.ErpCurrencySanityRow{Currency: l.Currency, Ok: true, Message: l.Period + ": " + itoa(l.Rows) + " rows"})
	}
	for _, o := range rep.Outliers {
		rows = append(rows, &financev1.ErpCurrencySanityRow{
			Currency: o.Currency, Rate: o.CostPerUnit.String(), Ok: false, Message: o.Period + " " + o.ErpItemCode + " " + o.ProductCode,
		})
	}
	return &financev1.GetErpCurrencySanityResponse{Base: erpOK("OK"), Data: rows}, nil
}

// GetErpItemLinkReport flattens the link-readiness report by category.
func (h *ErpIntegrationHandler) GetErpItemLinkReport(ctx context.Context, req *financev1.GetErpItemLinkReportRequest) (*financev1.GetErpItemLinkReportResponse, error) {
	rep, err := h.d.LinkReport.Handle(ctx, erpapp.LinkReadinessQuery{})
	if err != nil {
		return &financev1.GetErpItemLinkReportResponse{Base: erpErrBase(err)}, nil
	}
	rows := make([]*financev1.ErpLinkReportRow, 0, len(rep.NoProduct)+len(rep.AttributeGaps)+len(rep.Duplicates)+len(rep.LinkedNoShade)+len(rep.TypeMismatches))
	for _, l := range rep.NoProduct {
		rows = append(rows, &financev1.ErpLinkReportRow{Category: "no_product", ErpItemCode: l.ItemCode, Detail: l.ShadeCode})
	}
	for _, l := range rep.AttributeGaps {
		rows = append(rows, &financev1.ErpLinkReportRow{Category: "attribute_gap", ErpItemCode: l.ItemCode, ProductId: itoa(l.SysID), ProductCode: l.ProductCode})
	}
	for _, l := range rep.Duplicates {
		rows = append(rows, &financev1.ErpLinkReportRow{Category: "duplicate", ErpItemCode: l.ItemCode, Detail: l.ShadeCode})
	}
	for _, l := range rep.LinkedNoShade {
		rows = append(rows, &financev1.ErpLinkReportRow{Category: "linked_no_shade", ErpItemCode: l.ItemCode, ProductId: itoa(l.SysID), ProductCode: l.ProductCode})
	}
	for _, l := range rep.TypeMismatches {
		rows = append(rows, &financev1.ErpLinkReportRow{Category: "type_mismatch", ErpItemCode: l.ItemCode, ProductId: itoa(l.SysID), ProductCode: l.ProductCode, Detail: string(l.Direction)})
	}
	lo, hi, pg := erpPage(req.GetPagination(), len(rows))
	return &financev1.GetErpItemLinkReportResponse{Base: erpOK("OK"), Data: rows[lo:hi], Pagination: pg}, nil
}

// RunErpAttributeBackfill enqueues the attr_backfill job (dry_run as sent).
func (h *ErpIntegrationHandler) RunErpAttributeBackfill(ctx context.Context, req *financev1.RunErpAttributeBackfillRequest) (*financev1.RunErpAttributeBackfillResponse, error) {
	dry := req.GetDryRun()
	ex, err := h.d.Steps.TriggerSubtype(ctx, erpapp.SubtypeAttrBackfill, "", erpapp.AttrBackfillJobParams{DryRun: &dry}, erpActor(ctx))
	if err != nil {
		return &financev1.RunErpAttributeBackfillResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.RunErpAttributeBackfillResponse{Base: erpOK("Backfill queued"), JobId: ex.ID().String()}, nil
}

// RunErpBacktest enqueues the backtest job for a period.
func (h *ErpIntegrationHandler) RunErpBacktest(ctx context.Context, req *financev1.RunErpBacktestRequest) (*financev1.RunErpBacktestResponse, error) {
	ex, err := h.d.Steps.TriggerSubtype(ctx, erpapp.SubtypeBacktest, req.GetPeriod(), erpapp.BacktestJobParams{Period: req.GetPeriod()}, erpActor(ctx))
	if err != nil {
		return &financev1.RunErpBacktestResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.RunErpBacktestResponse{Base: erpOK("Backtest queued"), JobId: ex.ID().String()}, nil
}

// GetErpBacktestReport returns the stored backtest report of a batch.
func (h *ErpIntegrationHandler) GetErpBacktestReport(ctx context.Context, req *financev1.GetErpBacktestReportRequest) (*financev1.GetErpBacktestReportResponse, error) {
	rep, err := h.d.Backtest.Get(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.GetErpBacktestReportResponse{Base: erpErrBase(err)}, nil
	}
	counts := make(map[string]int64, len(rep.Counts))
	for k, v := range rep.Counts {
		counts[string(k)] = int64(v)
	}
	lo, hi, pg := erpPage(req.GetPagination(), len(rep.Lines))
	lines := make([]*financev1.ErpBacktestLine, 0, hi-lo)
	for _, l := range rep.Lines[lo:hi] {
		lines = append(lines, &financev1.ErpBacktestLine{
			ErpItemCode: l.Key.ItemCode, Basis: string(l.Basis),
			Class:     financev1.ErpBacktestClass(erpEnum(financev1.ErpBacktestClass_value, "ERP_BACKTEST_CLASS_", string(l.Class))),
			GoappsStd: erpNullDec(l.GoApps), LegacyStd: erpNullDec(l.Legacy), Diff: erpNullDec(l.Delta),
			DeltaPct: erpNullDec(l.DeltaPct), Fail: l.Fail,
		})
	}
	return &financev1.GetErpBacktestReportResponse{
		Base: erpOK("OK"), Pagination: pg,
		Data: &financev1.ErpBacktestReport{BatchId: req.GetBatchId(), Counts: counts, Lines: lines, Failed: rep.Failed},
	}, nil
}

// AbandonErpBatch marks a non-terminal batch FAILED (PG only, never Oracle).
func (h *ErpIntegrationHandler) AbandonErpBatch(ctx context.Context, req *financev1.AbandonErpBatchRequest) (*financev1.AbandonErpBatchResponse, error) {
	if h.d.Abandon == nil {
		return &financev1.AbandonErpBatchResponse{Base: InternalErrorResponse("abandon is not configured")}, nil
	}
	b, err := h.d.Abandon.Handle(ctx, erpapp.AbandonBatchCommand{
		BatchID: req.GetBatchId(), Reason: req.GetReason(), Actor: erpActor(ctx),
	})
	if err != nil {
		return &financev1.AbandonErpBatchResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.AbandonErpBatchResponse{Base: erpOK("ERP batch abandoned"), Data: erpBatchToProto(b)}, nil
}
