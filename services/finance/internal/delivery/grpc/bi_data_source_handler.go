package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	dsapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/datasource"
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

// Compile-time interface check.
var _ financev1.DataSourceServiceServer = (*BIDataSourceHandler)(nil)
