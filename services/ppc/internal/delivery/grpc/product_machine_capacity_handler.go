// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	capacityapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/capacity"
	capacitydomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/capacity"
)

// capacityHandler implements the product-machine-capacity RPCs of PPCService.
type capacityHandler struct {
	svc *capacityapp.Service
}

func newCapacityHandler(svc *capacityapp.Service) *capacityHandler {
	return &capacityHandler{svc: svc}
}

// CreateProductMachineCapacity creates a new product-machine capacity.
func (h *capacityHandler) CreateProductMachineCapacity(ctx context.Context, req *ppcv1.CreateProductMachineCapacityRequest) (*ppcv1.CreateProductMachineCapacityResponse, error) {
	prodPerDay, base := optionalDecimalField("prod_per_day", req.GetProdPerDay())
	if base != nil {
		return &ppcv1.CreateProductMachineCapacityResponse{Base: base}, nil
	}
	efficiencyPct, base := optionalDecimalField("efficiency_pct", req.GetEfficiencyPct())
	if base != nil {
		return &ppcv1.CreateProductMachineCapacityResponse{Base: base}, nil
	}

	entity, err := h.svc.Create(ctx, capacityapp.CreateCommand{
		CpmProductSysID: req.GetCpmProductSysId(),
		MachineID:       req.GetMachineId(),
		ProdPerDay:      prodPerDay,
		EfficiencyPct:   efficiencyPct,
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateProductMachineCapacityResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateProductMachineCapacityResponse{
		Base: successResponse("Product machine capacity created successfully"),
		Data: capacityToProto(entity),
	}, nil
}

// GetProductMachineCapacity retrieves a product-machine capacity by ID.
func (h *capacityHandler) GetProductMachineCapacity(ctx context.Context, req *ppcv1.GetProductMachineCapacityRequest) (*ppcv1.GetProductMachineCapacityResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetCapacityId())
	if err != nil {
		return &ppcv1.GetProductMachineCapacityResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetProductMachineCapacityResponse{
		Base: successResponse("Product machine capacity retrieved successfully"),
		Data: capacityToProto(entity),
	}, nil
}

// UpdateProductMachineCapacity updates an existing product-machine capacity.
func (h *capacityHandler) UpdateProductMachineCapacity(ctx context.Context, req *ppcv1.UpdateProductMachineCapacityRequest) (*ppcv1.UpdateProductMachineCapacityResponse, error) {
	prodPerDay, base := optionalDecimalField("prod_per_day", req.GetProdPerDay())
	if base != nil {
		return &ppcv1.UpdateProductMachineCapacityResponse{Base: base}, nil
	}
	efficiencyPct, base := optionalDecimalField("efficiency_pct", req.GetEfficiencyPct())
	if base != nil {
		return &ppcv1.UpdateProductMachineCapacityResponse{Base: base}, nil
	}

	entity, err := h.svc.Update(ctx, capacityapp.UpdateCommand{
		ID:            req.GetCapacityId(),
		ProdPerDay:    prodPerDay,
		EfficiencyPct: efficiencyPct,
		UpdatedBy:     getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdateProductMachineCapacityResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateProductMachineCapacityResponse{
		Base: successResponse("Product machine capacity updated successfully"),
		Data: capacityToProto(entity),
	}, nil
}

// DeleteProductMachineCapacity deletes a product-machine capacity.
func (h *capacityHandler) DeleteProductMachineCapacity(ctx context.Context, req *ppcv1.DeleteProductMachineCapacityRequest) (*ppcv1.DeleteProductMachineCapacityResponse, error) {
	if err := h.svc.Delete(ctx, req.GetCapacityId()); err != nil {
		return &ppcv1.DeleteProductMachineCapacityResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteProductMachineCapacityResponse{Base: successResponse("Product machine capacity deleted successfully")}, nil
}

// ListProductMachineCapacities lists product-machine capacities with filtering and pagination.
func (h *capacityHandler) ListProductMachineCapacities(ctx context.Context, req *ppcv1.ListProductMachineCapacitiesRequest) (*ppcv1.ListProductMachineCapacitiesResponse, error) {
	result, err := h.svc.List(ctx, capacityapp.ListQuery{
		Page:            int(req.GetPage()),
		PageSize:        int(req.GetPageSize()),
		CpmProductSysID: req.CpmProductSysId,
		MachineID:       req.MachineId,
		SortBy:          req.GetSortBy(),
		SortOrder:       req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListProductMachineCapacitiesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.ProductMachineCapacity, len(result.Items))
	for i, entity := range result.Items {
		items[i] = capacityToProto(entity)
	}
	return &ppcv1.ListProductMachineCapacitiesResponse{
		Base: successResponse("Product machine capacities retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func capacityToProto(e *capacitydomain.Capacity) *ppcv1.ProductMachineCapacity {
	proto := &ppcv1.ProductMachineCapacity{
		CapacityId:      e.ID(),
		CpmProductSysId: e.CpmProductSysID(),
		MachineId:       e.MachineID(),
		MachineNo:       e.MachineNo(),
		ProdPerDay:      formatOptionalDecimal(e.ProdPerDay()),
		EfficiencyPct:   formatOptionalDecimal(e.EfficiencyPct()),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
