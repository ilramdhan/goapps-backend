// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	commonlotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/commonlot"
	dpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dailyperf"
	etlapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
	commonlotdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/commonlot"
)

// packingHandler serves the Phase-3 packing & grading RPCs: grade-actual listing,
// common-lot CRUD, and the daily-performance Excel export.
type packingHandler struct {
	grades    *etlapp.GradeActualService
	commonLot *commonlotapp.Service
	exporter  *dpapp.Exporter
}

// newPackingHandler builds the packing sub-handler.
func newPackingHandler(grades *etlapp.GradeActualService, commonLot *commonlotapp.Service, exporter *dpapp.Exporter) *packingHandler {
	return &packingHandler{grades: grades, commonLot: commonLot, exporter: exporter}
}

// ListWOGradeActuals lists packing grade-actual lines, optionally scoped to one
// WO, grade, or department.
func (h *packingHandler) ListWOGradeActuals(ctx context.Context, req *ppcv1.ListWOGradeActualsRequest) (*ppcv1.ListWOGradeActualsResponse, error) {
	if h.grades == nil {
		return &ppcv1.ListWOGradeActualsResponse{Base: errorResponse(nilServiceCode, "grade actual service unavailable")}, nil
	}
	result, err := h.grades.List(ctx, etlapp.GradeActualQuery{
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		WOID:      req.WoId,
		Grade:     req.GetGrade(),
		Dept:      req.GetDept(),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListWOGradeActualsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	data := make([]*ppcv1.WOGradeActual, len(result.Items))
	for i := range result.Items {
		data[i] = gradeActualToProto(result.Items[i])
	}
	return &ppcv1.ListWOGradeActualsResponse{
		Base:       successResponse("Grade actuals retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// CreateCommonLot combines leftover bobbins from several original lots into a new
// ERP identity, tracking each original lot as a component.
func (h *packingHandler) CreateCommonLot(ctx context.Context, req *ppcv1.CreateCommonLotRequest) (*ppcv1.CreateCommonLotResponse, error) {
	if h.commonLot == nil {
		return &ppcv1.CreateCommonLotResponse{Base: errorResponse(nilServiceCode, "common lot service unavailable")}, nil
	}
	comps, errResp := commonLotComponentInputs(req.GetComponents())
	if errResp != nil {
		return &ppcv1.CreateCommonLotResponse{Base: errResp}, nil
	}
	lot, err := h.commonLot.Create(ctx, commonlotapp.CreateCommand{
		LotNo:        req.GetLotNo(),
		ItemCode:     req.GetItemCode(),
		ShadeCode:    req.GetShadeCode(),
		ErpGradeCode: req.GetErpGradeCode(),
		Components:   comps,
	})
	if err != nil {
		return &ppcv1.CreateCommonLotResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateCommonLotResponse{
		Base: successResponse("Common lot created successfully"),
		Data: commonLotToProto(lot),
	}, nil
}

// GetCommonLot returns a common lot with its component breakdown.
func (h *packingHandler) GetCommonLot(ctx context.Context, req *ppcv1.GetCommonLotRequest) (*ppcv1.GetCommonLotResponse, error) {
	if h.commonLot == nil {
		return &ppcv1.GetCommonLotResponse{Base: errorResponse(nilServiceCode, "common lot service unavailable")}, nil
	}
	lot, err := h.commonLot.Get(ctx, req.GetCommonLotId())
	if err != nil {
		return &ppcv1.GetCommonLotResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetCommonLotResponse{
		Base: successResponse("Common lot retrieved successfully"),
		Data: commonLotToProto(lot),
	}, nil
}

// ListCommonLots lists common lots with filtering and pagination.
func (h *packingHandler) ListCommonLots(ctx context.Context, req *ppcv1.ListCommonLotsRequest) (*ppcv1.ListCommonLotsResponse, error) {
	if h.commonLot == nil {
		return &ppcv1.ListCommonLotsResponse{Base: errorResponse(nilServiceCode, "common lot service unavailable")}, nil
	}
	result, err := h.commonLot.List(ctx, commonlotdomain.Filter{
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		Search:    req.GetSearch(),
		ItemCode:  req.GetItemCode(),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListCommonLotsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.CommonLot, len(result.Items))
	for i, lot := range result.Items {
		data[i] = commonLotToProto(lot)
	}
	return &ppcv1.ListCommonLotsResponse{
		Base:       successResponse("Common lots retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// ExportDailyPerformance exports the daily-performance efficiency snapshots for a
// date (all areas or one) to an .xlsx workbook.
func (h *packingHandler) ExportDailyPerformance(ctx context.Context, req *ppcv1.ExportDailyPerformanceRequest) (*ppcv1.ExportDailyPerformanceResponse, error) {
	if h.exporter == nil {
		return &ppcv1.ExportDailyPerformanceResponse{Base: errorResponse(nilServiceCode, "daily performance exporter unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.ExportDailyPerformanceResponse{Base: errResp}, nil
	}
	area := areaCodeToString(req.GetArea())
	content, err := h.exporter.ExportDailyPerformance(ctx, date, area)
	if err != nil {
		return &ppcv1.ExportDailyPerformanceResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	fileName := "daily-performance-" + req.GetDate()
	if area != "" {
		fileName += "-" + area
	}
	fileName += ".xlsx"
	return &ppcv1.ExportDailyPerformanceResponse{
		Base:        successResponse("Daily performance exported successfully"),
		FileContent: content,
		FileName:    fileName,
	}, nil
}

// ── proto mappers ────────────────────────────────────────────────────────────

// gradeActualToProto maps a grade-actual view to its proto message.
func gradeActualToProto(r etlapp.GradeActualView) *ppcv1.WOGradeActual {
	out := &ppcv1.WOGradeActual{
		GradeActualId: r.ID,
		WoId:          r.WOID,
		LotNo:         r.LotNo,
		Grade:         r.Grade,
		Dept:          r.Dept,
		TotalQtyKg:    formatDecimal(r.TotalQtyKg),
		BobbinCount:   r.BobbinCount,
		SyncedAt:      r.SyncedAt.Format(time.RFC3339),
	}
	if r.LastPackingDate != nil {
		out.LastPackingDate = formatDate(*r.LastPackingDate)
	}
	return out
}

// commonLotToProto maps a domain common lot to its proto message.
func commonLotToProto(l *commonlotdomain.CommonLot) *ppcv1.CommonLot {
	comps := l.Components()
	out := &ppcv1.CommonLot{
		CommonLotId:  l.ID(),
		LotNo:        l.LotNo(),
		ItemCode:     l.ItemCode(),
		ShadeCode:    l.ShadeCode(),
		ErpGradeCode: l.ErpGradeCode(),
		CreatedAt:    l.CreatedAt().Format(time.RFC3339),
		Components:   make([]*ppcv1.CommonLotComponent, len(comps)),
	}
	for i := range comps {
		out.Components[i] = commonLotComponentToProto(comps[i])
	}
	return out
}

// commonLotComponentToProto maps a domain component to its proto message.
func commonLotComponentToProto(c commonlotdomain.Component) *ppcv1.CommonLotComponent {
	return &ppcv1.CommonLotComponent{
		ComponentId:       c.ID(),
		CommonLotId:       c.CommonLotID(),
		OriginalLotNo:     c.OriginalLotNo(),
		OriginalShadeCode: c.OriginalShadeCode(),
		BobbinCount:       c.BobbinCount(),
		QtyKg:             formatDecimal(c.QtyKg()),
	}
}

// commonLotComponentInputs maps proto component inputs to application inputs,
// parsing the decimal-as-string quantity.
func commonLotComponentInputs(in []*ppcv1.CommonLotComponentInput) ([]commonlotapp.ComponentInput, *commonv1.BaseResponse) {
	out := make([]commonlotapp.ComponentInput, 0, len(in))
	for _, c := range in {
		qtyPtr, errResp := optionalDecimalField("qty_kg", c.GetQtyKg())
		if errResp != nil {
			return nil, errResp
		}
		var qty float64
		if qtyPtr != nil {
			qty = *qtyPtr
		}
		out = append(out, commonlotapp.ComponentInput{
			OriginalLotNo:     c.GetOriginalLotNo(),
			OriginalShadeCode: c.GetOriginalShadeCode(),
			BobbinCount:       c.GetBobbinCount(),
			QtyKg:             qty,
		})
	}
	return out, nil
}
