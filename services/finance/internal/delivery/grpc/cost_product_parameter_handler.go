package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cppapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// CostProductParameterHandler implements the CPP_ gRPC service.
type CostProductParameterHandler struct {
	financev1.UnimplementedCostProductParameterServiceServer
	app         *cppapp.Handlers
	override    *cppapp.OverrideParamValuesHandler
	editLogRepo cprapp.ParamEditLogByLevelReader
	paramRepo   parameter.Repository
}

// NewCostProductParameterHandler wires the handler.
func NewCostProductParameterHandler(app *cppapp.Handlers) *CostProductParameterHandler {
	return &CostProductParameterHandler{app: app}
}

// WithOverrideHandler attaches the optional param-value override handler.
func (h *CostProductParameterHandler) WithOverrideHandler(o *cppapp.OverrideParamValuesHandler) *CostProductParameterHandler {
	h.override = o
	return h
}

// WithEditLogRepo attaches the edit log reader used by ListParamEditLog.
func (h *CostProductParameterHandler) WithEditLogRepo(r cprapp.ParamEditLogByLevelReader) *CostProductParameterHandler {
	h.editLogRepo = r
	return h
}

// WithParamRepo attaches the parameter repository used by the MASTER_LOOKUP child-aware RPCs.
func (h *CostProductParameterHandler) WithParamRepo(r parameter.Repository) *CostProductParameterHandler {
	h.paramRepo = r
	return h
}

// ListParamEditLog returns the override audit history for one fill level of a CPR.
func (h *CostProductParameterHandler) ListParamEditLog(ctx context.Context, req *financev1.ListParamEditLogRequest) (*financev1.ListParamEditLogResponse, error) {
	if h.editLogRepo == nil {
		return &financev1.ListParamEditLogResponse{Base: InternalErrorResponse("edit log not configured")}, nil //nolint:nilerr // feature-not-configured surfaced via BaseResponse
	}
	entries, err := h.editLogRepo.ListByRequestLevel(ctx, req.RequestId, int(req.RouteLevel))
	if err != nil {
		return &financev1.ListParamEditLogResponse{Base: InternalErrorResponse("failed to load edit log")}, nil //nolint:nilerr // internal error surfaced via BaseResponse
	}
	out := make([]*financev1.ParamEditLogEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &financev1.ParamEditLogEntry{
			ParamCode: e.ParamCode,
			OldValue:  e.OldValue,
			NewValue:  e.NewValue,
			ChangedBy: e.ChangedBy,
			ChangedAt: e.ChangedAt,
		})
	}
	return &financev1.ListParamEditLogResponse{
		Base:    cppSuccessResponse("Edit log loaded"),
		Entries: out,
	}, nil
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
		return &financev1.DeleteProductParamValueResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
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
			LookupFillGroupCode:  m.LookupFillGroupCode,
		})
	}
	return &financev1.ListAvailableParamsResponse{Base: cppSuccessResponse("Available params loaded"), Data: out}, nil
}

// AddApplicableParam marks a param applicable for a product.
func (h *CostProductParameterHandler) AddApplicableParam(ctx context.Context, req *financev1.AddApplicableParamRequest) (*financev1.AddApplicableParamResponse, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return &financev1.AddApplicableParamResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
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
		return &financev1.RemoveApplicableParamResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
	}
	if err := h.app.RemoveApplicable(ctx, req.ProductSysId, paramID); err != nil {
		return &financev1.RemoveApplicableParamResponse{Base: cppDomainError(err)}, nil
	}
	return &financev1.RemoveApplicableParamResponse{Base: cppSuccessResponse("Parameter removed")}, nil
}

// OverrideParamValues lets an authorized user edit param values before the route is locked.
func (h *CostProductParameterHandler) OverrideParamValues(ctx context.Context, req *financev1.OverrideParamValuesRequest) (*financev1.OverrideParamValuesResponse, error) {
	if h.override == nil {
		return &financev1.OverrideParamValuesResponse{Base: InternalErrorResponse("override handler not configured")}, nil //nolint:nilerr // feature-not-configured surfaced via BaseResponse
	}
	items := make([]cppapp.OverrideParamItem, 0, len(req.Values))
	for _, v := range req.Values {
		paramID, err := uuid.Parse(v.ParamId)
		if err != nil {
			return &financev1.OverrideParamValuesResponse{Base: BadRequestResponse(fmt.Sprintf("invalid param_id %q", v.ParamId))}, nil //nolint:nilerr // invalid input surfaced via BaseResponse
		}
		item := cppapp.OverrideParamItem{
			ProductSysID: v.ProductSysId,
			ParamID:      paramID,
		}
		if v.ValueNumeric != "" {
			s := v.ValueNumeric
			item.ValueNumeric = &s
		}
		if v.ValueText != "" {
			s := v.ValueText
			item.ValueText = &s
		}
		if v.HasValueFlag {
			b := v.ValueFlag
			item.ValueFlag = &b
		}
		items = append(items, item)
	}
	actorID := getUserFromContext(ctx)
	count, err := h.override.Handle(ctx, cppapp.OverrideCommand{
		RequestID:  req.RequestId,
		RouteLevel: int(req.RouteLevel),
		Items:      items,
		ActorID:    actorID,
		ActorName:  actorID,
	})
	if err != nil {
		return &financev1.OverrideParamValuesResponse{Base: cppDomainError(err)}, nil //nolint:nilerr // domain error surfaced via BaseResponse
	}
	return &financev1.OverrideParamValuesResponse{
		Base:         cppSuccessResponse("Param values overridden"),
		UpdatedCount: int32(count), //nolint:gosec // count is bounded by len(req.Values) which is validated ≤ 100 via proto
	}, nil
}

// resolveChildIDs returns the UUIDs of all fill-group children for a MASTER_LOOKUP param.
// Returns an empty slice for non-MASTER_LOOKUP params or when paramRepo is not wired.
func (h *CostProductParameterHandler) resolveChildIDs(ctx context.Context, paramID uuid.UUID) ([]uuid.UUID, error) {
	if h.paramRepo == nil {
		return nil, nil
	}
	entity, err := h.paramRepo.GetByID(ctx, paramID)
	if err != nil {
		return nil, err
	}
	if entity.ParamCategory() != parameter.ParamCategoryMasterLookup {
		return nil, nil
	}
	children, err := h.paramRepo.GetByFillGroup(ctx, entity.Code().String())
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(children))
	for _, c := range children {
		ids = append(ids, c.ID())
	}
	return ids, nil
}

// AddApplicableParamWithChildren adds a MASTER_LOOKUP param + all its child params atomically.
func (h *CostProductParameterHandler) AddApplicableParamWithChildren(ctx context.Context, req *financev1.AddApplicableParamWithChildrenRequest) (*financev1.AddApplicableParamWithChildrenResponse, error) {
	paramID, err := uuid.Parse(req.GetParamId())
	if err != nil {
		return &financev1.AddApplicableParamWithChildrenResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // BaseResponse pattern
	}

	childIDs, childErr := h.resolveChildIDs(ctx, paramID)
	if childErr != nil {
		return &financev1.AddApplicableParamWithChildrenResponse{Base: cppDomainError(childErr)}, nil //nolint:nilerr // BaseResponse pattern
	}

	actor := getUserFromContext(ctx)
	if err = h.app.AddApplicableWithChildren(ctx, req.GetProductSysId(), paramID, actor, childIDs); err != nil {
		return &financev1.AddApplicableParamWithChildrenResponse{Base: cppDomainError(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	return &financev1.AddApplicableParamWithChildrenResponse{Base: cppSuccessResponse("Parameter and children added")}, nil
}

// GetRemoveApplicablePreview returns trigger + children for the confirm-delete dialog.
func (h *CostProductParameterHandler) GetRemoveApplicablePreview(ctx context.Context, req *financev1.GetRemoveApplicablePreviewRequest) (*financev1.GetRemoveApplicablePreviewResponse, error) {
	paramID, err := uuid.Parse(req.GetParamId())
	if err != nil {
		return &financev1.GetRemoveApplicablePreviewResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // BaseResponse pattern
	}

	preview, err := h.app.GetRemovePreview(ctx, req.GetProductSysId(), paramID)
	if err != nil {
		return &financev1.GetRemoveApplicablePreviewResponse{Base: cppDomainError(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	children := make([]*financev1.ParamWithValue, 0, len(preview.Children))
	for _, c := range preview.Children {
		children = append(children, &financev1.ParamWithValue{
			ParamCode:    c.ParamCode,
			ParamName:    c.ParamName,
			CurrentValue: c.CurrentValue,
		})
	}

	return &financev1.GetRemoveApplicablePreviewResponse{
		Base:             cppSuccessResponse("Remove preview loaded"),
		TriggerParamCode: preview.TriggerParamCode,
		TriggerParamName: preview.TriggerParamName,
		Children:         children,
	}, nil
}

// RemoveApplicableParamWithChildren removes a MASTER_LOOKUP param + all children + values atomically.
func (h *CostProductParameterHandler) RemoveApplicableParamWithChildren(ctx context.Context, req *financev1.RemoveApplicableParamWithChildrenRequest) (*financev1.RemoveApplicableParamWithChildrenResponse, error) {
	paramID, err := uuid.Parse(req.GetParamId())
	if err != nil {
		return &financev1.RemoveApplicableParamWithChildrenResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // BaseResponse pattern
	}
	actor := getUserFromContext(ctx)
	if err = h.app.RemoveApplicableWithChildren(ctx, req.GetProductSysId(), paramID, actor); err != nil {
		return &financev1.RemoveApplicableParamWithChildrenResponse{Base: cppDomainError(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	return &financev1.RemoveApplicableParamWithChildrenResponse{Base: cppSuccessResponse("Parameter and children removed")}, nil
}

// UpdateApplicableParam patches per-product override fields.
func (h *CostProductParameterHandler) UpdateApplicableParam(ctx context.Context, req *financev1.UpdateApplicableParamRequest) (*financev1.UpdateApplicableParamResponse, error) {
	paramID, err := uuid.Parse(req.ParamId)
	if err != nil {
		return &financev1.UpdateApplicableParamResponse{Base: BadRequestResponse("invalid param_id")}, nil //nolint:nilerr // invalid input surfaced via BaseResponse, gRPC error intentionally nil
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
		LookupFillGroupCode:  e.Meta.LookupFillGroupCode,
	}
	applyEntryValue(out, e.Value)
	return out
}

// applyEntryValue copies an optional filled value onto the proto entry.
func applyEntryValue(out *financev1.RequiredParamEntry, v *cpp.Value) {
	if v == nil {
		return
	}
	out.HasValue = true
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
	out.FilledBy = v.FilledBy
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
