// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	bfsapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/balanceforsale"
	dashapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dashboard"
	bfsdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/balanceforsale"
	dpdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
	dashdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dashboard"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// dashboardHandler serves the dashboard read RPCs: balance-for-sale, daily
// performance, morning review, and the efficiency-snapshot list.
type dashboardHandler struct {
	bfs      *bfsapp.Service
	dash     *dashapp.Service
	snapshot dpdomain.EfficiencySnapshotReader
}

// newDashboardHandler builds the dashboard sub-handler. dash and snapshot may be
// nil (the corresponding RPCs then report the service is unavailable).
func newDashboardHandler(bfs *bfsapp.Service, dash *dashapp.Service, snapshot dpdomain.EfficiencySnapshotReader) *dashboardHandler {
	return &dashboardHandler{bfs: bfs, dash: dash, snapshot: snapshot}
}

// GetBalanceForSale returns the per-product AX balance-for-sale breakdown.
// current_stock_AX is stubbed to 0 (no Orion inventory ETL in scope).
func (h *dashboardHandler) GetBalanceForSale(ctx context.Context, req *ppcv1.GetBalanceForSaleRequest) (*ppcv1.GetBalanceForSaleResponse, error) {
	rows, err := h.bfs.GetBalanceForSale(ctx, bfsapp.Query{
		CpmProductSysID:    req.CpmProductSysId,
		CommodityWatchOnly: req.GetCommodityWatchOnly(),
	})
	if err != nil {
		return &ppcv1.GetBalanceForSaleResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.BalanceForSaleRow, len(rows))
	for i := range rows {
		data[i] = balanceForSaleToProto(rows[i])
	}
	return &ppcv1.GetBalanceForSaleResponse{
		Base: successResponse("Balance for sale retrieved successfully"),
		Data: data,
	}, nil
}

// balanceForSaleToProto maps a domain balance row to its proto message.
func balanceForSaleToProto(r bfsdomain.Row) *ppcv1.BalanceForSaleRow {
	return &ppcv1.BalanceForSaleRow{
		CpmProductSysId:      r.CpmProductSysID,
		ProductCode:          r.ProductCode,
		ProductName:          r.ProductName,
		CurrentStockAx:       formatDecimal(r.CurrentStockAX),
		WoRunningOutputEst:   formatDecimal(r.WORunningOutputEst),
		MtsPlanQty:           formatDecimal(r.MtsPlanQty),
		CommittedContractQty: formatDecimal(r.CommittedContractQty),
		BalanceForSale:       formatDecimal(r.BalanceForSale),
	}
}

// GetDailyPerformance returns the daily-performance KPI cards (Today + MTD) and
// the monthly MC-EFF heatmap for the requested area and date. Figures are
// aggregated from AREA_DAY / MACHINE_SHIFT efficiency snapshots plus downtime
// (idle positions) and the area shift log (OT hours).
func (h *dashboardHandler) GetDailyPerformance(ctx context.Context, req *ppcv1.GetDailyPerformanceRequest) (*ppcv1.GetDailyPerformanceResponse, error) {
	if h.dash == nil {
		return &ppcv1.GetDailyPerformanceResponse{Base: errorResponse(nilServiceCode, "dashboard service unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.GetDailyPerformanceResponse{Base: errResp}, nil
	}
	view, err := h.dash.GetDailyPerformance(ctx, dashapp.DailyPerformanceQuery{
		Area:      areaCodeToString(req.GetArea()),
		Date:      date,
		Excluding: req.GetExcluding(),
	})
	if err != nil {
		return &ppcv1.GetDailyPerformanceResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	kpis := make([]*ppcv1.DailyPerformanceKpi, len(view.KPIs))
	for i := range view.KPIs {
		kpis[i] = kpiToProto(view.KPIs[i])
	}
	grid := make([]*ppcv1.McEffCell, len(view.McEffGrid))
	for i := range view.McEffGrid {
		grid[i] = mcEffCellToProto(view.McEffGrid[i])
	}
	return &ppcv1.GetDailyPerformanceResponse{
		Base:      successResponse("Daily performance retrieved successfully"),
		Kpis:      kpis,
		McEffGrid: grid,
	}, nil
}

// kpiToProto maps a domain KPI card to its proto message.
func kpiToProto(k dashdomain.KPI) *ppcv1.DailyPerformanceKpi {
	return &ppcv1.DailyPerformanceKpi{
		Key:        k.Key,
		Label:      k.Label,
		ValueToday: formatDecimal(k.ValueToday),
		ValueMtd:   formatDecimal(k.ValueMTD),
		Unit:       k.Unit,
	}
}

// mcEffCellToProto maps a domain MC-EFF cell to its proto message.
func mcEffCellToProto(c dashdomain.McEffCell) *ppcv1.McEffCell {
	return &ppcv1.McEffCell{
		MachineId: c.MachineID,
		MachineNo: c.MachineNo,
		Date:      c.Date.Format(time.DateOnly),
		Shift:     c.Shift,
		EffPct:    formatDecimal(c.EffPct),
	}
}

// GetMorningReview returns the morning-review dashboard: yesterday's actual vs
// plan per machine, open issues (pending-approval urgency), today's priorities,
// and headline quick stats.
func (h *dashboardHandler) GetMorningReview(ctx context.Context, req *ppcv1.GetMorningReviewRequest) (*ppcv1.GetMorningReviewResponse, error) {
	if h.dash == nil {
		return &ppcv1.GetMorningReviewResponse{Base: errorResponse(nilServiceCode, "dashboard service unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.GetMorningReviewResponse{Base: errResp}, nil
	}
	view, err := h.dash.GetMorningReview(ctx, dashapp.MorningReviewQuery{
		Area: areaCodeToString(req.GetArea()),
		Date: date,
	})
	if err != nil {
		return &ppcv1.GetMorningReviewResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	rows := make([]*ppcv1.MorningReviewMachineRow, len(view.ActualVsPlan))
	for i := range view.ActualVsPlan {
		rows[i] = morningRowToProto(view.ActualVsPlan[i])
	}
	issues := make([]*ppcv1.MorningReviewIssue, len(view.OpenIssues))
	for i := range view.OpenIssues {
		issues[i] = morningIssueToProto(view.OpenIssues[i])
	}
	priorities := make([]*ppcv1.MorningReviewPriority, len(view.Priorities))
	for i := range view.Priorities {
		priorities[i] = morningPriorityToProto(view.Priorities[i])
	}
	return &ppcv1.GetMorningReviewResponse{
		Base:               successResponse("Morning review retrieved successfully"),
		ActualVsPlan:       rows,
		OpenIssues:         issues,
		Priorities:         priorities,
		MachinesRunning:    view.Stats.MachinesRunning,
		MachinesTotal:      view.Stats.MachinesTotal,
		WosPendingApproval: view.Stats.WOsPendingApproval,
		UnmatchedSoCount:   view.Stats.UnmatchedSOCount,
	}, nil
}

// morningRowToProto maps a domain machine row to its proto message.
func morningRowToProto(r dashdomain.MachineRow) *ppcv1.MorningReviewMachineRow {
	return &ppcv1.MorningReviewMachineRow{
		MachineId:    r.MachineID,
		MachineNo:    r.MachineNo,
		Area:         stringToAreaCode(r.Area),
		QtyTarget:    formatDecimal(r.QtyTarget),
		QtyActual:    formatDecimal(r.QtyActual),
		VariancePct:  formatDecimal(r.VariancePct()),
		Flag:         r.Flag(),
		IsChangeover: r.IsChangeover,
	}
}

// morningIssueToProto maps a domain issue to its proto message.
func morningIssueToProto(i dashdomain.Issue) *ppcv1.MorningReviewIssue {
	return &ppcv1.MorningReviewIssue{
		IssueType: i.IssueType,
		RefId:     i.RefID,
		Title:     i.Title,
		Detail:    i.Detail,
		Severity:  i.Severity,
	}
}

// morningPriorityToProto maps a domain priority to its proto message.
func morningPriorityToProto(p dashdomain.Priority) *ppcv1.MorningReviewPriority {
	return &ppcv1.MorningReviewPriority{
		WoId:        p.WoID,
		TransNo:     p.WoNo,
		ProductCode: p.ProductCode,
		MachineNo:   p.MachineNo,
		Deadline:    p.Deadline.Format(time.DateOnly),
		QtyTarget:   formatDecimal(p.QtyTarget),
	}
}

// ListEfficiencySnapshots returns paginated efficiency snapshots filtered by
// area, scope, machine, and date range.
func (h *dashboardHandler) ListEfficiencySnapshots(ctx context.Context, req *ppcv1.ListEfficiencySnapshotsRequest) (*ppcv1.ListEfficiencySnapshotsResponse, error) {
	if h.snapshot == nil {
		return &ppcv1.ListEfficiencySnapshotsResponse{Base: errorResponse(nilServiceCode, "dashboard service unavailable")}, nil
	}
	filter := dpdomain.SnapshotFilter{
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		Area:      areaCodeToString(req.GetArea()),
		Scope:     req.GetScope(),
		MachineID: req.MachineId,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	}
	from, errResp := optionalDateField("date_from", req.GetDateFrom())
	if errResp != nil {
		return &ppcv1.ListEfficiencySnapshotsResponse{Base: errResp}, nil
	}
	to, errResp := optionalDateField("date_to", req.GetDateTo())
	if errResp != nil {
		return &ppcv1.ListEfficiencySnapshotsResponse{Base: errResp}, nil
	}
	filter.DateFrom = from
	filter.DateTo = to

	snaps, total, err := h.snapshot.ListSnapshots(ctx, filter)
	if err != nil {
		return &ppcv1.ListEfficiencySnapshotsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.EfficiencySnapshot, len(snaps))
	for i := range snaps {
		data[i] = efficiencySnapshotToProto(snaps[i])
	}
	page, pageSize := req.GetPage(), req.GetPageSize()
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &ppcv1.ListEfficiencySnapshotsResponse{
		Base:       successResponse("Efficiency snapshots retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(page, pageSize, total, totalPages),
	}, nil
}

// efficiencySnapshotToProto maps a domain efficiency snapshot to its proto
// message.
func efficiencySnapshotToProto(s *dpdomain.EfficiencySnapshot) *ppcv1.EfficiencySnapshot {
	out := &ppcv1.EfficiencySnapshot{
		SnapshotId:         s.ID,
		Area:               stringToAreaCode(s.Area),
		Scope:              s.Scope,
		Date:               s.Date.Format(time.DateOnly),
		IsExcluding:        s.IsExcluding,
		QtyTheoretical_100: formatDecimal(s.QtyTheoretical100),
		QtyTheoreticalRng:  formatDecimal(s.QtyTheoreticalRng),
		QtyLoss:            formatDecimal(s.QtyLoss),
		QtyWaste:           formatDecimal(s.QtyWaste),
		QtyActual:          formatDecimal(s.QtyActual),
		EffProductionPct:   formatDecimal(s.EffProductionPct),
		EffRunningPct:      formatDecimal(s.EffRunningPct),
		EffPlantPct:        formatDecimal(s.EffPlantPct),
		YieldPct:           formatDecimal(s.YieldPct),
		WastePct:           formatDecimal(s.WastePct),
		BreaksCount:        s.BreaksCount,
		BreaksPerTon:       formatDecimal(s.BreaksPerTon),
		CalcAt:             s.CalcAt.Format(time.RFC3339),
	}
	if s.MachineID != nil {
		out.MachineId = *s.MachineID
	}
	if s.WoID != nil {
		out.WoId = *s.WoID
	}
	if s.Shift != nil {
		out.Shift = *s.Shift
	}
	if s.Segment != nil {
		out.Segment = *s.Segment
	}
	return out
}

// compile-time interface guard.
var _ interface {
	GetBalanceForSale(context.Context, *ppcv1.GetBalanceForSaleRequest) (*ppcv1.GetBalanceForSaleResponse, error)
	GetDailyPerformance(context.Context, *ppcv1.GetDailyPerformanceRequest) (*ppcv1.GetDailyPerformanceResponse, error)
	GetMorningReview(context.Context, *ppcv1.GetMorningReviewRequest) (*ppcv1.GetMorningReviewResponse, error)
	ListEfficiencySnapshots(context.Context, *ppcv1.ListEfficiencySnapshotsRequest) (*ppcv1.ListEfficiencySnapshotsResponse, error)
} = (*dashboardHandler)(nil)
