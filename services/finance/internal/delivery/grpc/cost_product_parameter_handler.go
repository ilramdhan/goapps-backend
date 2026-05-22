package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cppapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// CostProductParameterHandler implements the CPP_ gRPC service.
type CostProductParameterHandler struct {
	financev1.UnimplementedCostProductParameterServiceServer
	app *cppapp.Handlers
}

// NewCostProductParameterHandler wires the handler.
func NewCostProductParameterHandler(app *cppapp.Handlers) *CostProductParameterHandler {
	return &CostProductParameterHandler{app: app}
}

// ListProductRequiredParams returns form contents per product.
func (h *CostProductParameterHandler) ListProductRequiredParams(ctx context.Context, req *financev1.ListProductRequiredParamsRequest) (*financev1.ListProductRequiredParamsResponse, error) {
	entries, err := h.app.ListProductRequiredParams(ctx, req.ProductSysId, req.RequiredOnly)
	if err != nil {
		return &financev1.ListProductRequiredParamsResponse{Base: cppDomainError(err)}, nil
	}
	out := make([]*financev1.RequiredParamEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, requiredEntryToProto(e))
	}
	return &financev1.ListProductRequiredParamsResponse{
		Base: cppSuccessResponse("Product required params loaded"),
		Data: out,
	}, nil
}

// UpsertProductParamValue writes a single value.
func (h *CostProductParameterHandler) UpsertProductParamValue(ctx context.Context, req *financev1.UpsertProductParamValueRequest) (*financev1.UpsertProductParamValueResponse, error) {
	cmd, err := upsertReqToCommand(ctx, req)
	if err != nil {
		return &financev1.UpsertProductParamValueResponse{Base: BadRequestResponse(err.Error())}, nil
	}
	v, err := h.app.Upsert(ctx, cmd)
	if err != nil {
		return &financev1.UpsertProductParamValueResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.UpsertProductParamValueResponse{
		Base: cppSuccessResponse("Parameter value saved"),
		Data: valueToProto(v),
	}, nil
}

// UpsertProductParamValuesBatch writes multiple values, non-atomically.
func (h *CostProductParameterHandler) UpsertProductParamValuesBatch(ctx context.Context, req *financev1.UpsertProductParamValuesBatchRequest) (*financev1.UpsertProductParamValuesBatchResponse, error) {
	cmds := make([]cppapp.UpsertCommand, 0, len(req.Values))
	for _, v := range req.Values {
		cmd, err := upsertReqToCommand(ctx, v)
		if err != nil {
			return &financev1.UpsertProductParamValuesBatchResponse{Base: BadRequestResponse(err.Error())}, nil
		}
		cmds = append(cmds, cmd)
	}
	res, err := h.app.UpsertBatch(ctx, req.ProductSysId, cmds)
	if err != nil {
		return &financev1.UpsertProductParamValuesBatchResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.UpsertProductParamValuesBatchResponse{
		Base:             cppSuccessResponse(fmt.Sprintf("%d values saved", res.UpsertedCount)),
		UpsertedCount:    res.UpsertedCount,
		FailedCount:      res.FailedCount,
		FailedParamCodes: res.FailedParamCodes,
	}, nil
}

// DeleteProductParamValue clears one value.
func (h *CostProductParameterHandler) DeleteProductParamValue(ctx context.Context, req *financev1.DeleteProductParamValueRequest) (*financev1.DeleteProductParamValueResponse, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return &financev1.DeleteProductParamValueResponse{Base: BadRequestResponse("invalid param_id")}, nil
	}
	if err := h.app.Delete(ctx, req.ProductSysId, paramID); err != nil {
		return &financev1.DeleteProductParamValueResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.DeleteProductParamValueResponse{Base: cppSuccessResponse("Parameter value cleared")}, nil
}

// CheckMissingRequiredParams returns the still-unbound required params.
func (h *CostProductParameterHandler) CheckMissingRequiredParams(ctx context.Context, req *financev1.CheckMissingRequiredParamsRequest) (*financev1.CheckMissingRequiredParamsResponse, error) {
	missing, err := h.app.CheckMissing(ctx, req.ProductSysId)
	if err != nil {
		return &financev1.CheckMissingRequiredParamsResponse{Base: cppDomainError(err)}, nil
	}
	out := make([]*financev1.MissingParam, 0, len(missing))
	for _, m := range missing {
		out = append(out, &financev1.MissingParam{
			ParamId:      m.ParamID.String(),
			ParamCode:    m.ParamCode,
			ParamName:    m.ParamName,
			DisplayGroup: m.DisplayGroup,
		})
	}
	return &financev1.CheckMissingRequiredParamsResponse{
		Base: cppSuccessResponse("Missing params computed"),
		Data: out,
	}, nil
}

// ListAvailableParams returns params NOT yet applicable for the product.
func (h *CostProductParameterHandler) ListAvailableParams(ctx context.Context, req *financev1.ListAvailableParamsRequest) (*financev1.ListAvailableParamsResponse, error) {
	metas, err := h.app.ListAvailable(ctx, req.ProductSysId)
	if err != nil {
		return &financev1.ListAvailableParamsResponse{Base: cppDomainError(err)}, nil
	}
	out := make([]*financev1.AvailableParamEntry, 0, len(metas))
	for _, m := range metas {
		out = append(out, &financev1.AvailableParamEntry{
			ParamId:              m.ParamID.String(),
			ParamCode:            m.ParamCode,
			ParamName:            m.ParamName,
			ParamShortName:       m.ParamShortName,
			DataType:             m.DataType,
			ParamCategory:        m.ParamCategory,
			UomCode:              m.UOMCode,
			OwnerDepartment:      m.OwnerDepartment,
			IsRequiredForCosting: m.IsRequiredForCosting,
			LookupMasterCode:     m.LookupMasterCode,
			DisplayOrder:         m.DisplayOrder,
			DisplayGroup:         m.DisplayGroup,
		})
	}
	return &financev1.ListAvailableParamsResponse{Base: cppSuccessResponse("Available params loaded"), Data: out}, nil
}

// AddApplicableParam marks a param applicable for a product.
func (h *CostProductParameterHandler) AddApplicableParam(ctx context.Context, req *financev1.AddApplicableParamRequest) (*financev1.AddApplicableParamResponse, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return &financev1.AddApplicableParamResponse{Base: BadRequestResponse("invalid param_id")}, nil
	}
	var displayOrder *int32
	if req.DisplayOrder > 0 {
		d := req.DisplayOrder
		displayOrder = &d
	}
	if err := h.app.AddApplicable(ctx, req.ProductSysId, paramID, req.IsRequired, displayOrder, getUserFromContext(ctx)); err != nil {
		return &financev1.AddApplicableParamResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.AddApplicableParamResponse{Base: cppSuccessResponse("Parameter added")}, nil
}

// RemoveApplicableParam removes a param from a product.
func (h *CostProductParameterHandler) RemoveApplicableParam(ctx context.Context, req *financev1.RemoveApplicableParamRequest) (*financev1.RemoveApplicableParamResponse, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return &financev1.RemoveApplicableParamResponse{Base: BadRequestResponse("invalid param_id")}, nil
	}
	if err := h.app.RemoveApplicable(ctx, req.ProductSysId, paramID); err != nil {
		return &financev1.RemoveApplicableParamResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.RemoveApplicableParamResponse{Base: cppSuccessResponse("Parameter removed")}, nil
}

// UpdateApplicableParam patches per-product override fields.
func (h *CostProductParameterHandler) UpdateApplicableParam(ctx context.Context, req *financev1.UpdateApplicableParamRequest) (*financev1.UpdateApplicableParamResponse, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return &financev1.UpdateApplicableParamResponse{Base: BadRequestResponse("invalid param_id")}, nil
	}
	if err := h.app.UpdateApplicable(ctx, req.ProductSysId, paramID, req.IsRequired, req.DisplayOrder, getUserFromContext(ctx)); err != nil {
		return &financev1.UpdateApplicableParamResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.UpdateApplicableParamResponse{Base: cppSuccessResponse("Parameter applicability updated")}, nil
}

// =============================================================================
// helpers
// =============================================================================

func upsertReqToCommand(ctx context.Context, req *financev1.UpsertProductParamValueRequest) (cppapp.UpsertCommand, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return cppapp.UpsertCommand{}, errors.New("invalid param_id")
	}
	cmd := cppapp.UpsertCommand{
		ProductSysID: req.ProductSysId,
		ParamID:      paramID,
		FilledBy:     getUserFromContext(ctx),
	}
	if req.ValueNumeric != "" {
		s := req.ValueNumeric
		cmd.ValueNumeric = &s
	}
	if req.ValueText != "" {
		s := req.ValueText
		cmd.ValueText = &s
	}
	if req.HasValueFlag {
		b := req.ValueFlag
		cmd.ValueFlag = &b
	}
	return cmd, nil
}

func requiredEntryToProto(e cpp.RequiredEntry) *financev1.RequiredParamEntry {
	out := &financev1.RequiredParamEntry{
		ParamId:              e.Meta.ParamID.String(),
		ParamCode:            e.Meta.ParamCode,
		ParamName:            e.Meta.ParamName,
		ParamShortName:       e.Meta.ParamShortName,
		DataType:             e.Meta.DataType,
		ParamCategory:        e.Meta.ParamCategory,
		UomCode:              e.Meta.UOMCode,
		OwnerDepartment:      e.Meta.OwnerDepartment,
		IsRequiredForCosting: e.Meta.IsRequiredForCosting,
		LookupMasterCode:     e.Meta.LookupMasterCode,
		DisplayOrder:         e.Meta.DisplayOrder,
		DisplayGroup:         e.Meta.DisplayGroup,
	}
	if e.Value != nil {
		out.HasValue = true
		if e.Value.ValueNumeric != nil {
			out.ValueNumeric = *e.Value.ValueNumeric
		}
		if e.Value.ValueText != nil {
			out.ValueText = *e.Value.ValueText
		}
		if e.Value.ValueFlag != nil {
			out.ValueFlag = *e.Value.ValueFlag
		}
		if !e.Value.FilledAt.IsZero() {
			out.FilledAt = e.Value.FilledAt.Format("2006-01-02T15:04:05Z07:00")
		}
		out.FilledBy = e.Value.FilledBy
	}
	return out
}

func valueToProto(v *cpp.Value) *financev1.CostProductParameterValue {
	out := &financev1.CostProductParameterValue{
		ValueId:      v.ValueID,
		ProductSysId: v.ProductSysID,
		ParamId:      v.ParamID.String(),
		FilledBy:     v.FilledBy,
	}
	if v.ValueNumeric != nil {
		out.ValueNumeric = *v.ValueNumeric
	}
	if v.ValueText != nil {
		out.ValueText = *v.ValueText
	}
	if v.ValueFlag != nil {
		out.ValueFlag = *v.ValueFlag
	}
	if !v.FilledAt.IsZero() {
		out.FilledAt = v.FilledAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}

func cppSuccessResponse(msg string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{IsSuccess: true, StatusCode: "200", Message: msg}
}

func cppDomainError(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, cpp.ErrProductNotFound), errors.Is(err, cpp.ErrParamNotFound), errors.Is(err, cpp.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, cpp.ErrInvalidValueShape), errors.Is(err, cpp.ErrInvalidDataType), errors.Is(err, cpp.ErrPeriodDependent), errors.Is(err, cpp.ErrParamNotApplicable):
		return BadRequestResponse(err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
