package grpc

import (
	"context"
	"errors"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// CostMasterLookupHandler implements financev1.CostMasterLookupServiceServer.
// It serves read-only finance master projections to the PPC service (the
// CostMasterLookupService gRPC contract). All methods are additive and never
// mutate finance state. Note: this is unrelated to LookupMasterHandler, which
// serves the mst_lookup enum-label master.
type CostMasterLookupHandler struct {
	financev1.UnimplementedCostMasterLookupServiceServer
	repo       *postgres.CostMasterLookupRepository
	resolver   *costproductmaster.ResolveByErpCodeHandler
	validation *ValidationHelper
}

// NewCostMasterLookupHandler constructs the handler.
func NewCostMasterLookupHandler(repo *postgres.CostMasterLookupRepository) (*CostMasterLookupHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostMasterLookupHandler{
		repo:       repo,
		resolver:   costproductmaster.NewResolveByErpCodeHandler(repo),
		validation: v,
	}, nil
}

// GetCostProductMasterForPPC returns one product projection by sys id.
func (h *CostMasterLookupHandler) GetCostProductMasterForPPC(ctx context.Context, req *financev1.GetCostProductMasterForPPCRequest) (*financev1.GetCostProductMasterForPPCResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductMasterForPPCResponse{Base: baseResp}, nil
	}
	p, err := h.repo.GetProduct(ctx, req.GetProductSysId())
	if err != nil {
		if errors.Is(err, postgres.ErrLookupNotFound) {
			return &financev1.GetCostProductMasterForPPCResponse{Base: NotFoundResponse("product not found")}, nil
		}
		return &financev1.GetCostProductMasterForPPCResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	return &financev1.GetCostProductMasterForPPCResponse{
		Base: successResponse("OK"),
		Data: lookupProductToProto(*p),
	}, nil
}

// BatchGetCostProductMaster resolves many product sys ids at once.
func (h *CostMasterLookupHandler) BatchGetCostProductMaster(ctx context.Context, req *financev1.BatchGetCostProductMasterRequest) (*financev1.BatchGetCostProductMasterResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.BatchGetCostProductMasterResponse{Base: baseResp}, nil
	}
	products, err := h.repo.BatchGetProducts(ctx, req.GetProductSysIds())
	if err != nil {
		return &financev1.BatchGetCostProductMasterResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	data := make([]*financev1.CostMasterProduct, 0, len(products))
	for i := range products {
		data = append(data, lookupProductToProto(products[i]))
	}
	return &financev1.BatchGetCostProductMasterResponse{
		Base: successResponse("OK"),
		Data: data,
	}, nil
}

// ResolveCostProductMasterByErpCode resolves (erp_item_code, shade_code) pairs
// to product projections, reporting ambiguity explicitly via match_count.
func (h *CostMasterLookupHandler) ResolveCostProductMasterByErpCode(ctx context.Context, req *financev1.ResolveCostProductMasterByErpCodeRequest) (*financev1.ResolveCostProductMasterByErpCodeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ResolveCostProductMasterByErpCodeResponse{Base: baseResp}, nil
	}
	reqPairs := req.GetPairs()
	pairs := make([]costproductmaster.ErpCodePair, 0, len(reqPairs))
	for _, p := range reqPairs {
		pairs = append(pairs, costproductmaster.ErpCodePair{
			ErpItemCode: p.GetErpItemCode(),
			ShadeCode:   p.GetShadeCode(),
		})
	}
	resolutions, err := h.resolver.Handle(ctx, pairs)
	if err != nil {
		return &financev1.ResolveCostProductMasterByErpCodeResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	data := make([]*financev1.ErpCodeResolution, 0, len(resolutions))
	for i := range resolutions {
		r := resolutions[i]
		item := &financev1.ErpCodeResolution{
			Pair: &financev1.ErpCodePair{
				ErpItemCode: r.Pair.ErpItemCode,
				ShadeCode:   r.Pair.ShadeCode,
			},
			MatchCount: r.MatchCount,
		}
		if r.Product != nil {
			item.Product = lookupProductToProto(*r.Product)
		}
		data = append(data, item)
	}
	return &financev1.ResolveCostProductMasterByErpCodeResponse{
		Base:        successResponse("OK"),
		Resolutions: data,
	}, nil
}

// ListCostProductMasterForPPC lists product projections for pickers.
func (h *CostMasterLookupHandler) ListCostProductMasterForPPC(ctx context.Context, req *financev1.ListCostProductMasterForPPCRequest) (*financev1.ListCostProductMasterForPPCResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostProductMasterForPPCResponse{Base: baseResp}, nil
	}
	page, pageSize := req.GetPage(), req.GetPageSize()
	products, total, err := h.repo.ListProducts(ctx, page, pageSize, req.GetSearch(), req.GetProductTypeId(), req.GetShadeCode(), req.GetActiveFilter())
	if err != nil {
		return &financev1.ListCostProductMasterForPPCResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	data := make([]*financev1.CostMasterProduct, 0, len(products))
	for i := range products {
		data = append(data, lookupProductToProto(products[i]))
	}
	return &financev1.ListCostProductMasterForPPCResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// GetProductRouteForPPC returns the released route projection for a product.
func (h *CostMasterLookupHandler) GetProductRouteForPPC(ctx context.Context, req *financev1.GetProductRouteForPPCRequest) (*financev1.GetProductRouteForPPCResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetProductRouteForPPCResponse{Base: baseResp}, nil
	}
	head, stages, rms, err := h.repo.GetReleasedRoute(ctx, req.GetProductSysId())
	if err != nil {
		return &financev1.GetProductRouteForPPCResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	if head == nil {
		return &financev1.GetProductRouteForPPCResponse{Base: NotFoundResponse("no released route for product")}, nil
	}
	return &financev1.GetProductRouteForPPCResponse{
		Base: successResponse("OK"),
		Data: lookupRouteToProto(head, stages, rms),
	}, nil
}

// ListProductGradesForPPC lists product-grade projections for pickers.
func (h *CostMasterLookupHandler) ListProductGradesForPPC(ctx context.Context, req *financev1.ListProductGradesForPPCRequest) (*financev1.ListProductGradesForPPCResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListProductGradesForPPCResponse{Base: baseResp}, nil
	}
	page, pageSize := req.GetPage(), req.GetPageSize()
	grades, total, err := h.repo.ListGrades(ctx, page, pageSize, req.GetSearch(), req.GetActiveFilter())
	if err != nil {
		return &financev1.ListProductGradesForPPCResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	data := make([]*financev1.CostMasterGrade, 0, len(grades))
	for i := range grades {
		g := grades[i]
		data = append(data, &financev1.CostMasterGrade{
			PgId:         g.PgID,
			PgCode:       g.PgCode,
			PgName:       g.PgName,
			PgGradeLabel: g.PgGradeLabel,
			IsActive:     g.IsActive,
		})
	}
	return &financev1.ListProductGradesForPPCResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// ListProductParametersForPPC lists mst_parameter definitions (by display_group).
func (h *CostMasterLookupHandler) ListProductParametersForPPC(ctx context.Context, req *financev1.ListProductParametersForPPCRequest) (*financev1.ListProductParametersForPPCResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListProductParametersForPPCResponse{Base: baseResp}, nil
	}
	page, pageSize := req.GetPage(), req.GetPageSize()
	defs, total, err := h.repo.ListParameterDefs(ctx, page, pageSize, req.GetSearch(), req.GetDisplayGroup(), req.GetActiveFilter())
	if err != nil {
		return &financev1.ListProductParametersForPPCResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	data := make([]*financev1.CostMasterProductParameterDef, 0, len(defs))
	for i := range defs {
		data = append(data, lookupParamDefToProto(defs[i]))
	}
	return &financev1.ListProductParametersForPPCResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// BatchGetProductParameterValues returns per-product typed param values.
func (h *CostMasterLookupHandler) BatchGetProductParameterValues(ctx context.Context, req *financev1.BatchGetProductParameterValuesRequest) (*financev1.BatchGetProductParameterValuesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.BatchGetProductParameterValuesResponse{Base: baseResp}, nil
	}
	values, err := h.repo.BatchGetParameterValues(ctx, req.GetProductSysIds(), req.GetParamIds())
	if err != nil {
		return &financev1.BatchGetProductParameterValuesResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	data := make([]*financev1.CostMasterProductParameterValue, 0, len(values))
	for i := range values {
		v := values[i]
		data = append(data, &financev1.CostMasterProductParameterValue{
			ProductSysId: v.ProductSysID,
			ParamId:      v.ParamID,
			ParamCode:    v.ParamCode,
			DataType:     v.DataType,
			ValueNumeric: v.ValueNumeric,
			ValueText:    v.ValueText,
			ValueFlag:    v.ValueFlag,
		})
	}
	return &financev1.BatchGetProductParameterValuesResponse{
		Base: successResponse("OK"),
		Data: data,
	}, nil
}

// =============================================================================
// mappers
// =============================================================================

// lookupProductToProto maps a flat product projection to proto. product_denier
// is intentionally left empty (Gap G-005: no denier column on cost_product_master).
func lookupProductToProto(p postgres.LookupProduct) *financev1.CostMasterProduct {
	return &financev1.CostMasterProduct{
		ProductSysId:    p.ProductSysID,
		ProductCode:     p.ProductCode,
		ProductTypeId:   p.ProductTypeID,
		ProductTypeCode: p.ProductTypeCode,
		ProductTypeName: p.ProductTypeName,
		ProductName:     p.ProductName,
		ShadeCode:       p.ShadeCode,
		ShadeName:       p.ShadeName,
		GradeCode:       p.GradeCode,
		ErpItemCode:     p.ErpItemCode,
		ErpGradeCode_1:  p.ErpGradeCode1,
		ErpGradeCode_2:  p.ErpGradeCode2,
		IsActive:        p.IsActive,
		ProductDenier:   "",
	}
}

func lookupParamDefToProto(d postgres.LookupParamDef) *financev1.CostMasterProductParameterDef {
	return &financev1.CostMasterProductParameterDef{
		ParamId:              d.ParamID,
		ParamCode:            d.ParamCode,
		ParamName:            d.ParamName,
		ParamShortName:       d.ParamShortName,
		DataType:             d.DataType,
		ParamCategory:        d.ParamCategory,
		DisplayGroup:         d.DisplayGroup,
		LookupMasterCode:     d.LookupMasterCode,
		UomId:                d.UomID,
		UomCode:              d.UomCode,
		DefaultValue:         d.DefaultValue,
		MinValue:             d.MinValue,
		MaxValue:             d.MaxValue,
		DisplayOrder:         d.DisplayOrder,
		IsRequiredForCosting: d.IsRequiredForCosting,
		IsActive:             d.IsActive,
	}
}

func lookupRouteToProto(head *postgres.LookupRouteHead, stages []postgres.LookupRouteStage, rms []postgres.LookupRouteRm) *financev1.CostMasterRoute {
	// Group RM edges by their owning stage seq id.
	rmsBySeq := make(map[int64][]*financev1.CostMasterRouteRm, len(stages))
	for i := range rms {
		rm := rms[i]
		rmsBySeq[rm.SeqID] = append(rmsBySeq[rm.SeqID], &financev1.CostMasterRouteRm{
			RmId:           rm.RmID,
			SeqId:          rm.SeqID,
			RmType:         rm.RmType,
			RmProductSysId: rm.RmProductSysID,
			RmItemCode:     rm.RmItemCode,
			RmGroupCode:    rm.RmGroupCode,
			RouteRmRatio:   rm.RouteRmRatio,
			SubType:        rm.SubType,
		})
	}
	stageProtos := make([]*financev1.CostMasterRouteStage, 0, len(stages))
	for i := range stages {
		s := stages[i]
		stageProtos = append(stageProtos, &financev1.CostMasterRouteStage{
			SeqId:          s.SeqID,
			RouteLevel:     s.RouteLevel,
			RouteSeq:       s.RouteSeq,
			RouteName:      s.RouteName,
			RouteItemCode:  s.RouteItemCode,
			RouteShadeCode: s.RouteShadeCode,
			Rms:            rmsBySeq[s.SeqID],
			// The product the stage produces — the anchor a multi-level route
			// walk uses to attach a stage's RM edges to their downstream item.
			StageProductSysId: s.ProductSysID,
		})
	}
	return &financev1.CostMasterRoute{
		HeadId:        head.HeadID,
		ProductSysId:  head.ProductSysID,
		ProductCode:   head.ProductCode,
		RoutingStatus: head.RoutingStatus,
		Version:       head.Version,
		Stages:        stageProtos,
	}
}

// ensure interface compliance.
var _ financev1.CostMasterLookupServiceServer = (*CostMasterLookupHandler)(nil)
