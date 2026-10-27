package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	chartdataapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dsapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/datasource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// BIDataSourceHandler implements financev1.DataSourceServiceServer.
type BIDataSourceHandler struct {
	financev1.UnimplementedDataSourceServiceServer
	listHandler      *dsapp.ListHandler
	distinctsHandler *dsapp.GetDistinctsHandler
	validationHelper *ValidationHelper
}

// NewBIDataSourceHandler constructs the gRPC handler.
func NewBIDataSourceHandler(list *dsapp.ListHandler, distincts *dsapp.GetDistinctsHandler) (*BIDataSourceHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BIDataSourceHandler{
		listHandler:      list,
		distinctsHandler: distincts,
		validationHelper: v,
	}, nil
}

// ListDataSources returns the data-source registry.
func (h *BIDataSourceHandler) ListDataSources(ctx context.Context, req *financev1.ListDataSourcesRequest) (*financev1.ListDataSourcesResponse, error) {
	out, err := h.listHandler.Handle(ctx, req.GetIncludeInactive())
	if err != nil {
		return &financev1.ListDataSourcesResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.DataSource, 0, len(out))
	for _, ds := range out {
		items = append(items, dataSourceToProto(ds))
	}
	return &financev1.ListDataSourcesResponse{
		Base: successResponse("Data sources listed"),
		Data: items,
	}, nil
}

// GetFactDistincts returns distinct type/group_1/2/3 values for admin form dropdowns.
func (h *BIDataSourceHandler) GetFactDistincts(ctx context.Context, req *financev1.GetFactDistinctsRequest) (*financev1.GetFactDistinctsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetFactDistinctsResponse{Base: baseResp}, nil
	}
	d, err := h.distinctsHandler.Handle(ctx, dsapp.GetDistinctsQuery{Type: req.GetType()})
	if err != nil {
		return &financev1.GetFactDistinctsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	// HD-07/HD-10: HR_* metadata is hidden from callers without workforce.view; group_3 (NIK)
	// additionally needs the employee-level permission.
	if !IsSuperAdmin(ctx) {
		hideHRDistincts(ctx, req.GetType(), &d)
	}
	return &financev1.GetFactDistinctsResponse{
		Base: successResponse("Distincts retrieved"),
		Data: &financev1.FactMetricDistinct{
			Types:         d.Types,
			Group_1S:      d.Group1s,
			Group_2S:      d.Group2s,
			Group_3S:      d.Group3s,
			DimensionKeys: d.DimensionKeys,
		},
	}, nil
}

// hideHRDistincts strips HR_* metadata the caller may not see (HD-07/HD-10).
func hideHRDistincts(ctx context.Context, typ string, d *factmetric.DistinctValues) {
	canView := HasPermission(ctx, chartdataapp.PermWorkforceView)
	if !canView {
		d.Types = filterOutHRTypes(d.Types)
	}
	if !chartdataapp.IsHRType(typ) {
		return
	}
	if !canView {
		d.Group1s, d.Group2s, d.Group3s, d.DimensionKeys = nil, nil, nil, nil
	} else if !HasPermission(ctx, chartdataapp.PermWorkforceEmployee) {
		d.Group3s = nil
	}
}

// filterOutHRTypes drops HR_* type names so callers without workforce.view cannot discover them.
func filterOutHRTypes(types []string) []string {
	out := make([]string, 0, len(types))
	for _, t := range types {
		if !chartdataapp.IsHRType(t) {
			out = append(out, t)
		}
	}
	return out
}

// Compile-time interface check.
var _ financev1.DataSourceServiceServer = (*BIDataSourceHandler)(nil)
