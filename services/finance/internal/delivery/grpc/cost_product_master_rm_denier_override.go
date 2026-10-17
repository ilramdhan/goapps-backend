package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// WithRMDenierOverrides wires the repository backing the RM denier override RPCs.
func (h *CostProductMasterHandler) WithRMDenierOverrides(r domain.RMDenierOverrideRepository) {
	h.overrideRepo = r
}

// rmDenierOverrideErrToBase maps override domain errors to BaseResponse: validation
// sentinels become 400 with a field-level validation error, missing entities 404.
func rmDenierOverrideErrToBase(err error) *commonv1.BaseResponse {
	field := func(f string) *commonv1.BaseResponse {
		b := ErrorResponse("400", "Validation failed")
		b.ValidationErrors = []*commonv1.ValidationError{{Field: f, Message: err.Error()}}
		return b
	}
	switch {
	case errors.Is(err, domain.ErrNotFound),
		errors.Is(err, domain.ErrOverrideRMProductNotFound),
		errors.Is(err, domain.ErrOverrideRMGroupNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidOverrideRMType):
		return field("rm_type")
	case errors.Is(err, domain.ErrInvalidOverrideRMIdentity):
		return field("rm_identity")
	case errors.Is(err, domain.ErrInvalidOverrideDenier):
		return field("denier")
	case errors.Is(err, domain.ErrInvalidOverrideReason):
		return field("reason")
	default:
		return InternalErrorResponse(err.Error())
	}
}

func rmDenierOverrideToProto(o *domain.RMDenierOverride) *financev1.RMDenierOverride {
	ts := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	return &financev1.RMDenierOverride{
		Id:             o.ID(),
		ProductSysId:   o.ProductSysID(),
		RmType:         o.RMType(),
		RmProductSysId: o.RMProductSysID(),
		RmProductCode:  o.RMProductCode(),
		RmProductName:  o.RMProductName(),
		RmGroupCode:    o.RMGroupCode(),
		RmGroupName:    o.RMGroupName(),
		Denier:         o.Denier(),
		Reason:         o.Reason(),
		CreatedAt:      ts(o.CreatedAt()),
		CreatedBy:      o.CreatedBy(),
		UpdatedAt:      ts(o.UpdatedAt()),
		UpdatedBy:      o.UpdatedBy(),
	}
}

// emitOverrideAudit records an override change on the owner product, with the RM
// identity (and denier/reason/propagation) in the JSON payload. Best-effort.
func (h *CostProductMasterHandler) emitOverrideAudit(ctx context.Context, productSysID int64, actor string, before, after map[string]any) {
	if h.auditRepo == nil {
		return
	}
	enc := func(m map[string]any) string {
		if m == nil {
			return ""
		}
		b, err := json.Marshal(m)
		if err != nil {
			return ""
		}
		return string(b)
	}
	if err := h.auditRepo.Emit(ctx, costauditlog.NewInput{
		EntityType: "cost_product_master",
		EntityID:   productSysID,
		Operation:  costauditlog.OpUpdate,
		BeforeData: enc(before),
		AfterData:  enc(after),
		UserID:     actor,
	}); err != nil {
		_ = err // best-effort: audit never blocks a mutation
	}
}

func overrideIdentity(rmType string, rmProductSysID int64, rmGroupCode string) map[string]any {
	return map[string]any{"kind": "rm_denier_override", "rm_type": rmType, "rm_product_sys_id": rmProductSysID, "rm_group_code": rmGroupCode}
}

var errOverrideUnavailable = errors.New("rm denier override repository not configured")

// ListRMDenierOverrides lists a product's overrides plus its direct duplicates.
func (h *CostProductMasterHandler) ListRMDenierOverrides(ctx context.Context, req *financev1.ListRMDenierOverridesRequest) (*financev1.ListRMDenierOverridesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListRMDenierOverridesResponse{Base: baseResp}, nil
	}
	if h.overrideRepo == nil {
		return &financev1.ListRMDenierOverridesResponse{Base: InternalErrorResponse(errOverrideUnavailable.Error())}, nil
	}
	list, err := h.overrideRepo.ListRMDenierOverrides(ctx, req.GetProductSysId())
	if err != nil {
		return &financev1.ListRMDenierOverridesResponse{Base: rmDenierOverrideErrToBase(err)}, nil
	}
	dups, err := h.overrideRepo.ListDirectDuplicates(ctx, req.GetProductSysId())
	if err != nil {
		return &financev1.ListRMDenierOverridesResponse{Base: rmDenierOverrideErrToBase(err)}, nil
	}
	resp := &financev1.ListRMDenierOverridesResponse{Base: successResponse("OK")}
	for _, o := range list {
		resp.Data = append(resp.Data, rmDenierOverrideToProto(o))
	}
	for _, d := range dups {
		resp.DuplicateProducts = append(resp.DuplicateProducts, &financev1.DuplicateProductRef{
			ProductSysId: d.ProductSysID, ProductCode: d.ProductCode, ProductName: d.ProductName,
		})
	}
	return resp, nil
}

// UpsertRMDenierOverride creates or updates an override, optionally on direct duplicates.
func (h *CostProductMasterHandler) UpsertRMDenierOverride(ctx context.Context, req *financev1.UpsertRMDenierOverrideRequest) (*financev1.UpsertRMDenierOverrideResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpsertRMDenierOverrideResponse{Base: baseResp}, nil
	}
	if h.overrideRepo == nil {
		return &financev1.UpsertRMDenierOverrideResponse{Base: InternalErrorResponse(errOverrideUnavailable.Error())}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	o, err := domain.NewRMDenierOverride(domain.RMDenierOverrideInput{
		ProductSysID:   req.GetProductSysId(),
		RMType:         req.GetRmType(),
		RMProductSysID: req.GetRmProductSysId(),
		RMGroupCode:    req.GetRmGroupCode(),
		Denier:         req.GetDenier(),
		Reason:         req.GetReason(),
		ActorUserID:    actor,
	})
	if err != nil {
		return &financev1.UpsertRMDenierOverrideResponse{Base: rmDenierOverrideErrToBase(err)}, nil
	}
	saved, applied, err := h.overrideRepo.UpsertRMDenierOverride(ctx, o, req.GetApplyToDuplicates(), actor)
	if err != nil {
		return &financev1.UpsertRMDenierOverrideResponse{Base: rmDenierOverrideErrToBase(err)}, nil
	}
	after := overrideIdentity(saved.RMType(), saved.RMProductSysID(), saved.RMGroupCode())
	after["op"] = "upsert"
	after["denier"] = saved.Denier()
	after["reason"] = saved.Reason()
	after["applied_product_sys_ids"] = applied
	h.emitOverrideAudit(ctx, saved.ProductSysID(), actor, nil, after)
	return &financev1.UpsertRMDenierOverrideResponse{
		Base:                 successResponse("RM denier override saved"),
		Data:                 rmDenierOverrideToProto(saved),
		AppliedProductSysIds: applied,
	}, nil
}

// DeleteRMDenierOverride removes an override, optionally on direct duplicates.
func (h *CostProductMasterHandler) DeleteRMDenierOverride(ctx context.Context, req *financev1.DeleteRMDenierOverrideRequest) (*financev1.DeleteRMDenierOverrideResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteRMDenierOverrideResponse{Base: baseResp}, nil
	}
	if h.overrideRepo == nil {
		return &financev1.DeleteRMDenierOverrideResponse{Base: InternalErrorResponse(errOverrideUnavailable.Error())}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	rmType, rmProd, rmGroup, err := domain.ValidateRMIdentity(req.GetProductSysId(), req.GetRmType(), req.GetRmProductSysId(), req.GetRmGroupCode())
	if err != nil {
		return &financev1.DeleteRMDenierOverrideResponse{Base: rmDenierOverrideErrToBase(err)}, nil
	}
	deleted, applied, err := h.overrideRepo.DeleteRMDenierOverride(ctx, domain.DeleteRMDenierOverrideInput{
		ProductSysID:      req.GetProductSysId(),
		RMType:            rmType,
		RMProductSysID:    rmProd,
		RMGroupCode:       rmGroup,
		ApplyToDuplicates: req.GetApplyToDuplicates(),
		ActorUserID:       actor,
	})
	if err != nil {
		return &financev1.DeleteRMDenierOverrideResponse{Base: rmDenierOverrideErrToBase(err)}, nil
	}
	before := overrideIdentity(rmType, rmProd, rmGroup)
	before["op"] = "delete"
	before["deleted_count"] = deleted
	before["applied_product_sys_ids"] = applied
	h.emitOverrideAudit(ctx, req.GetProductSysId(), actor, before, nil)
	return &financev1.DeleteRMDenierOverrideResponse{
		Base:                 successResponse("RM denier override deleted"),
		DeletedCount:         int32(deleted), //nolint:gosec // bounded by 1 + direct duplicates
		AppliedProductSysIds: applied,
	}, nil
}
