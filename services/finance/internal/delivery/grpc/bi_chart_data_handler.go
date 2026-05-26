package grpc

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	chartdataapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
)

// BIChartDataHandler implements financev1.ChartDataServiceServer.
type BIChartDataHandler struct {
	financev1.UnimplementedChartDataServiceServer
	getDataHandler   *chartdataapp.GetDataHandler
	previewHandler   *chartdataapp.PreviewHandler
	validationHelper *ValidationHelper
}

// NewBIChartDataHandler constructs the gRPC handler.
func NewBIChartDataHandler(get *chartdataapp.GetDataHandler, preview *chartdataapp.PreviewHandler) (*BIChartDataHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BIChartDataHandler{
		getDataHandler:   get,
		previewHandler:   preview,
		validationHelper: v,
	}, nil
}

// GetDashboardData resolves a dashboard + viewer filters into a chart payload.
func (h *BIChartDataHandler) GetDashboardData(ctx context.Context, req *financev1.GetDashboardDataRequest) (*financev1.GetDashboardDataResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetDashboardDataResponse{Base: baseResp}, nil
	}
	filters := chartdataapp.ViewerFilters{
		PeriodPreset: req.GetPeriodPreset(),
		Compare:      compareModeToString(req.GetCompare()),
		DrillPath:    req.GetDrillPath(),
	}
	if req.GetPeriodFrom() != nil {
		filters.PeriodFrom = req.GetPeriodFrom().AsTime()
	}
	if req.GetPeriodTo() != nil {
		filters.PeriodTo = req.GetPeriodTo().AsTime()
	}
	q := chartdataapp.GetDataQuery{
		DashboardCode: req.GetDashboardCode(),
		Filters:       filters,
		UserRoles:     GetRolesFromCtx(ctx),
		IsSuperAdmin:  IsSuperAdmin(ctx),
	}
	result, err := h.getDataHandler.Handle(ctx, q)
	if err != nil {
		return &financev1.GetDashboardDataResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.GetDashboardDataResponse{
		Base: successResponse("Chart data retrieved"),
		Data: chartResultToProto(result),
	}, nil
}

// PreviewDashboard renders an unsaved dashboard config.
func (h *BIChartDataHandler) PreviewDashboard(ctx context.Context, req *financev1.PreviewDashboardRequest) (*financev1.PreviewDashboardResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.PreviewDashboardResponse{Base: baseResp}, nil
	}
	modes := make([]string, 0, len(req.GetCompareModes()))
	for _, m := range req.GetCompareModes() {
		modes = append(modes, compareModeToString(m))
	}
	q := chartdataapp.PreviewQuery{
		FilterType:   req.GetFilterType(),
		FilterGroup1: req.GetFilterGroup_1(),
		PeriodGrain:  periodeGrainToString(req.GetPeriodeGrain()),
		ChartType:    chartTypeToString(req.GetChartType()),
		ChartConfig:  structToMap(req.GetChartConfig()),
		KpiConfig:    structListToMaps(req.GetKpiConfig()),
		CompareModes: modes,
	}
	result, err := h.previewHandler.Handle(ctx, q)
	if err != nil {
		return &financev1.PreviewDashboardResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.PreviewDashboardResponse{
		Base: successResponse("Preview rendered"),
		Data: chartResultToProto(result),
	}, nil
}

// chartResultToProto converts the application-layer Result into the proto envelope.
func chartResultToProto(r *chartdataapp.Result) *financev1.ChartDataResponse {
	if r == nil {
		return nil
	}
	series := make([]*financev1.Series, 0, len(r.Series))
	for _, s := range r.Series {
		points := make([]*financev1.DataPoint, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, &financev1.DataPoint{
				Category: p.Category,
				Value:    p.Value,
				Label:    p.Label,
				Meta:     mapToStruct(p.Meta),
			})
		}
		series = append(series, &financev1.Series{
			Name:    s.Name,
			LibHint: s.LibHint,
			Points:  points,
		})
	}
	kpis := make([]*financev1.KpiResult, 0, len(r.KPIs))
	for _, k := range r.KPIs {
		kpis = append(kpis, &financev1.KpiResult{
			Label:              k.Label,
			Value:              k.Value,
			ValueFormatted:     k.ValueFormatted,
			CompareValue:       k.CompareValue,
			DeltaAbs:           k.DeltaAbs,
			DeltaPct:           k.DeltaPct,
			ComparePeriodLabel: k.ComparePeriodLabel,
			Improving:          k.Improving,
			Sparkline:          k.Sparkline,
		})
	}
	meta := &financev1.Meta{
		RowCount:  int32(r.Meta.RowCount), //nolint:gosec
		CacheHit:  r.Meta.CacheHit,
		QueryHash: r.Meta.QueryHash,
	}
	if !r.Meta.AsOf.IsZero() {
		meta.AsOf = timestamppb.New(r.Meta.AsOf)
	} else {
		meta.AsOf = timestamppb.New(time.Now().UTC())
	}
	return &financev1.ChartDataResponse{
		Config:     mapToStruct(r.Config),
		Series:     series,
		Categories: r.Categories,
		Kpis:       kpis,
		DrillContext: &financev1.DrillContext{
			CurrentPath: r.DrillContext.CurrentPath,
			NextField:   r.DrillContext.NextField,
			NextValues:  r.DrillContext.NextValues,
			CanDrill:    r.DrillContext.CanDrill,
		},
		Meta: meta,
	}
}

// Compile-time interface check.
var _ financev1.ChartDataServiceServer = (*BIChartDataHandler)(nil)

// Silence unused-import (commonv1 used elsewhere in this package).
var _ = commonv1.BaseResponse{}
