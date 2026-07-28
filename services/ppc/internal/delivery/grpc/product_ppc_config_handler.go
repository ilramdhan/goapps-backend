// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	productconfigapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/productconfig"
	configdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/productconfig"
)

// productConfigHandler implements the product-config RPCs of PPCService.
type productConfigHandler struct {
	svc *productconfigapp.Service
}

func newProductConfigHandler(svc *productconfigapp.Service) *productConfigHandler {
	return &productConfigHandler{svc: svc}
}

// CreateProductPPCConfig creates a new product PPC config.
func (h *productConfigHandler) CreateProductPPCConfig(ctx context.Context, req *ppcv1.CreateProductPPCConfigRequest) (*ppcv1.CreateProductPPCConfigResponse, error) {
	cmd := productconfigapp.CreateCommand{
		CpmProductSysID:  req.GetCpmProductSysId(),
		IsCommodityWatch: req.GetIsCommodityWatch(),
		MachineGroupID:   req.MachineGroupId,
		CreatedBy:        getUserFromContext(ctx),
	}

	fields := []struct {
		name string
		val  string
		dst  **float64
	}{
		{"price_sell", req.GetPriceSell(), &cmd.PriceSell},
		{"yield_std", req.GetYieldStd(), &cmd.YieldStd},
		{"buffer_rm_pct", req.GetBufferRmPct(), &cmd.BufferRmPct},
		{"ax_yield_pct", req.GetAxYieldPct(), &cmd.AxYieldPct},
		{"denier", req.GetDenier(), &cmd.Denier},
	}
	for _, f := range fields {
		v, base := optionalDecimalField(f.name, f.val)
		if base != nil {
			return &ppcv1.CreateProductPPCConfigResponse{Base: base}, nil
		}
		*f.dst = v
	}

	entity, err := h.svc.Create(ctx, cmd)
	if err != nil {
		return &ppcv1.CreateProductPPCConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateProductPPCConfigResponse{
		Base: successResponse("Product config created successfully"),
		Data: productConfigToProto(entity),
	}, nil
}

// GetProductPPCConfig retrieves a product PPC config by ID.
func (h *productConfigHandler) GetProductPPCConfig(ctx context.Context, req *ppcv1.GetProductPPCConfigRequest) (*ppcv1.GetProductPPCConfigResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetConfigId())
	if err != nil {
		return &ppcv1.GetProductPPCConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetProductPPCConfigResponse{
		Base: successResponse("Product config retrieved successfully"),
		Data: productConfigToProto(entity),
	}, nil
}

// UpdateProductPPCConfig updates an existing product PPC config.
func (h *productConfigHandler) UpdateProductPPCConfig(ctx context.Context, req *ppcv1.UpdateProductPPCConfigRequest) (*ppcv1.UpdateProductPPCConfigResponse, error) {
	cmd := productconfigapp.UpdateCommand{
		ID:               req.GetConfigId(),
		IsCommodityWatch: req.IsCommodityWatch,
		MachineGroupID:   req.MachineGroupId,
		UpdatedBy:        getUserFromContext(ctx),
	}

	fields := []struct {
		name string
		src  *string
		dst  **float64
	}{
		{"price_sell", req.PriceSell, &cmd.PriceSell},
		{"yield_std", req.YieldStd, &cmd.YieldStd},
		{"buffer_rm_pct", req.BufferRmPct, &cmd.BufferRmPct},
		{"ax_yield_pct", req.AxYieldPct, &cmd.AxYieldPct},
		{"denier", req.Denier, &cmd.Denier},
	}
	for _, f := range fields {
		if f.src == nil {
			continue
		}
		v, base := optionalDecimalField(f.name, *f.src)
		if base != nil {
			return &ppcv1.UpdateProductPPCConfigResponse{Base: base}, nil
		}
		*f.dst = v
	}

	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdateProductPPCConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateProductPPCConfigResponse{
		Base: successResponse("Product config updated successfully"),
		Data: productConfigToProto(entity),
	}, nil
}

// DeleteProductPPCConfig deletes a product PPC config.
func (h *productConfigHandler) DeleteProductPPCConfig(ctx context.Context, req *ppcv1.DeleteProductPPCConfigRequest) (*ppcv1.DeleteProductPPCConfigResponse, error) {
	if err := h.svc.Delete(ctx, req.GetConfigId()); err != nil {
		return &ppcv1.DeleteProductPPCConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteProductPPCConfigResponse{Base: successResponse("Product config deleted successfully")}, nil
}

// ListProductPPCConfigs lists product PPC configs with filtering and pagination.
func (h *productConfigHandler) ListProductPPCConfigs(ctx context.Context, req *ppcv1.ListProductPPCConfigsRequest) (*ppcv1.ListProductPPCConfigsResponse, error) {
	result, err := h.svc.List(ctx, productconfigapp.ListQuery{
		Page:               int(req.GetPage()),
		PageSize:           int(req.GetPageSize()),
		Search:             req.GetSearch(),
		CommodityWatchOnly: req.CommodityWatchOnly,
		SortBy:             req.GetSortBy(),
		SortOrder:          req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListProductPPCConfigsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.ProductPPCConfig, len(result.Items))
	for i, entity := range result.Items {
		items[i] = productConfigToProto(entity)
	}
	return &ppcv1.ListProductPPCConfigsResponse{
		Base: successResponse("Product configs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func productConfigToProto(e *configdomain.ProductConfig) *ppcv1.ProductPPCConfig {
	proto := &ppcv1.ProductPPCConfig{
		ConfigId:         e.ID(),
		CpmProductSysId:  e.CpmProductSysID(),
		ProductCode:      "", // TODO(plan-03c): resolve product_code/product_name via financeclient
		ProductName:      "", // TODO(plan-03c): resolve product_code/product_name via financeclient
		IsCommodityWatch: e.IsCommodityWatch(),
		PriceSell:        formatOptionalDecimal(e.PriceSell()),
		YieldStd:         formatOptionalDecimal(e.YieldStd()),
		BufferRmPct:      formatOptionalDecimal(e.BufferRmPct()),
		AxYieldPct:       formatOptionalDecimal(e.AxYieldPct()),
		Denier:           formatOptionalDecimal(e.Denier()),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.MachineGroupID() != nil {
		proto.MachineGroupId = *e.MachineGroupID()
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
