// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	app "github.com/mutugading/goapps-backend/services/ppc/internal/application/productmachineparameter"
	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/productmachineparameter"
)

// productMachineParameterHandler implements the product-machine-parameter RPCs
// of PPCService.
type productMachineParameterHandler struct {
	svc *app.Service
}

func newProductMachineParameterHandler(svc *app.Service) *productMachineParameterHandler {
	return &productMachineParameterHandler{svc: svc}
}

// CreateProductMachineParameter creates a new product-machine parameter value.
func (h *productMachineParameterHandler) CreateProductMachineParameter(ctx context.Context, req *ppcv1.CreateProductMachineParameterRequest) (*ppcv1.CreateProductMachineParameterResponse, error) {
	valueNum, base := optionalDecimalField("value_num", req.GetValueNum())
	if base != nil {
		return &ppcv1.CreateProductMachineParameterResponse{Base: base}, nil
	}
	valueText := optionalStringField(req.GetValueText())
	valueFlag := optionalBoolField(req.GetValueFlag(), req.GetHasValueFlag())

	entity, err := h.svc.Create(ctx, app.CreateCommand{
		CpmProductSysID: req.GetCpmProductSysId(),
		MachineID:       req.GetMachineId(),
		ParamID:         req.GetParamId(),
		ValueNum:        valueNum,
		ValueText:       valueText,
		ValueFlag:       valueFlag,
	})
	if err != nil {
		return &ppcv1.CreateProductMachineParameterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateProductMachineParameterResponse{
		Base: successResponse("Product machine parameter created successfully"),
		Data: productMachineParameterToProto(entity),
	}, nil
}

// GetProductMachineParameter retrieves a product-machine parameter by ID.
func (h *productMachineParameterHandler) GetProductMachineParameter(ctx context.Context, req *ppcv1.GetProductMachineParameterRequest) (*ppcv1.GetProductMachineParameterResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetPmpId())
	if err != nil {
		return &ppcv1.GetProductMachineParameterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetProductMachineParameterResponse{
		Base: successResponse("Product machine parameter retrieved successfully"),
		Data: productMachineParameterToProto(entity),
	}, nil
}

// UpdateProductMachineParameter updates the typed value of an existing parameter.
func (h *productMachineParameterHandler) UpdateProductMachineParameter(ctx context.Context, req *ppcv1.UpdateProductMachineParameterRequest) (*ppcv1.UpdateProductMachineParameterResponse, error) {
	valueNum, base := optionalDecimalField("value_num", req.GetValueNum())
	if base != nil {
		return &ppcv1.UpdateProductMachineParameterResponse{Base: base}, nil
	}

	entity, err := h.svc.Update(ctx, app.UpdateCommand{
		ID:        req.GetPmpId(),
		ValueNum:  valueNum,
		ValueText: req.ValueText,
		ValueFlag: req.ValueFlag,
	})
	if err != nil {
		return &ppcv1.UpdateProductMachineParameterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateProductMachineParameterResponse{
		Base: successResponse("Product machine parameter updated successfully"),
		Data: productMachineParameterToProto(entity),
	}, nil
}

// DeleteProductMachineParameter deletes a product-machine parameter.
func (h *productMachineParameterHandler) DeleteProductMachineParameter(ctx context.Context, req *ppcv1.DeleteProductMachineParameterRequest) (*ppcv1.DeleteProductMachineParameterResponse, error) {
	if err := h.svc.Delete(ctx, req.GetPmpId()); err != nil {
		return &ppcv1.DeleteProductMachineParameterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteProductMachineParameterResponse{Base: successResponse("Product machine parameter deleted successfully")}, nil
}

// ListProductMachineParameters lists product-machine parameters with filtering and pagination.
func (h *productMachineParameterHandler) ListProductMachineParameters(ctx context.Context, req *ppcv1.ListProductMachineParametersRequest) (*ppcv1.ListProductMachineParametersResponse, error) {
	result, err := h.svc.List(ctx, app.ListQuery{
		Page:            int(req.GetPage()),
		PageSize:        int(req.GetPageSize()),
		CpmProductSysID: req.CpmProductSysId,
		MachineID:       req.MachineId,
		ParamID:         req.GetParamId(),
		SortBy:          req.GetSortBy(),
		SortOrder:       req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListProductMachineParametersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.ProductMachineParameter, len(result.Items))
	for i, entity := range result.Items {
		items[i] = productMachineParameterToProto(entity)
	}
	return &ppcv1.ListProductMachineParametersResponse{
		Base: successResponse("Product machine parameters retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func productMachineParameterToProto(e *domain.Parameter) *ppcv1.ProductMachineParameter {
	proto := &ppcv1.ProductMachineParameter{
		PmpId:           e.ID(),
		CpmProductSysId: e.CpmProductSysID(),
		MachineId:       e.MachineID(),
		MachineNo:       e.MachineNo(),
		ParamId:         e.ParamID(),
		ValueNum:        formatOptionalDecimal(e.ValueNum()),
	}
	if e.ValueText() != nil {
		proto.ValueText = *e.ValueText()
	}
	if e.ValueFlag() != nil {
		proto.ValueFlag = *e.ValueFlag()
	}
	if e.UpdatedAt() != nil {
		proto.Audit = &commonv1.AuditInfo{
			UpdatedAt: e.UpdatedAt().Format(time.RFC3339),
		}
	}
	return proto
}
