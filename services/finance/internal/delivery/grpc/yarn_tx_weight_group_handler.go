// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appytwg "github.com/mutugading/goapps-backend/services/finance/internal/application/yarntxweightgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// Metric operation labels for the TX Weight group service.
const (
	ytwgOpCreate = "create"
	ytwgOpGet    = "get"
	ytwgOpUpdate = "update"
	ytwgOpDelete = "delete"
	ytwgOpList   = "list"
	ytwgIDField  = "group_id"
)

// YarnTxWeightGroupHandler implements financev1.YarnTxWeightGroupServiceServer.
type YarnTxWeightGroupHandler struct {
	financev1.UnimplementedYarnTxWeightGroupServiceServer
	createHandler *appytwg.CreateHandler
	getHandler    *appytwg.GetHandler
	listHandler   *appytwg.ListHandler
	updateHandler *appytwg.UpdateHandler
	deleteHandler *appytwg.DeleteHandler
	validation    *ValidationHelper
}

// NewYarnTxWeightGroupHandler constructs a YarnTxWeightGroupHandler.
func NewYarnTxWeightGroupHandler(repo yarntxweightgroup.Repository) (*YarnTxWeightGroupHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &YarnTxWeightGroupHandler{
		createHandler: appytwg.NewCreateHandler(repo),
		getHandler:    appytwg.NewGetHandler(repo),
		listHandler:   appytwg.NewListHandler(repo),
		updateHandler: appytwg.NewUpdateHandler(repo),
		deleteHandler: appytwg.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateYarnTxWeightGroup creates a group with its product types and grade rules.
func (h *YarnTxWeightGroupHandler) CreateYarnTxWeightGroup(ctx context.Context, req *financev1.CreateYarnTxWeightGroupRequest) (*financev1.CreateYarnTxWeightGroupResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpCreate, false)
		return &financev1.CreateYarnTxWeightGroupResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appytwg.CreateCommand{
		Input:     ytwgInput(req.Code, req.Name, req.Description, req.ProductTypeIds, req.Rules),
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpCreate, false)
		return &financev1.CreateYarnTxWeightGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightGroupOperation(ytwgOpCreate, true)
	return &financev1.CreateYarnTxWeightGroupResponse{
		Base: successResponse("TX weight group created successfully"),
		Data: yarnTxWeightGroupToProto(entity),
	}, nil
}

// GetYarnTxWeightGroup retrieves a group by ID.
func (h *YarnTxWeightGroupHandler) GetYarnTxWeightGroup(ctx context.Context, req *financev1.GetYarnTxWeightGroupRequest) (*financev1.GetYarnTxWeightGroupResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpGet, false)
		return &financev1.GetYarnTxWeightGroupResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GroupId)
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpGet, false)
		return &financev1.GetYarnTxWeightGroupResponse{Base: invalidIDResponse(ytwgIDField)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appytwg.GetQuery{ID: id})
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpGet, false)
		return &financev1.GetYarnTxWeightGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightGroupOperation(ytwgOpGet, true)
	return &financev1.GetYarnTxWeightGroupResponse{
		Base: successResponse("TX weight group retrieved successfully"),
		Data: yarnTxWeightGroupToProto(entity),
	}, nil
}

// UpdateYarnTxWeightGroup replaces the header, product-type set and rules of a group.
func (h *YarnTxWeightGroupHandler) UpdateYarnTxWeightGroup(ctx context.Context, req *financev1.UpdateYarnTxWeightGroupRequest) (*financev1.UpdateYarnTxWeightGroupResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpUpdate, false)
		return &financev1.UpdateYarnTxWeightGroupResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GroupId)
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpUpdate, false)
		return &financev1.UpdateYarnTxWeightGroupResponse{Base: invalidIDResponse(ytwgIDField)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.updateHandler.Handle(ctx, appytwg.UpdateCommand{
		ID:        id,
		Input:     ytwgInput(req.Code, req.Name, req.Description, req.ProductTypeIds, req.Rules),
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpUpdate, false)
		return &financev1.UpdateYarnTxWeightGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightGroupOperation(ytwgOpUpdate, true)
	return &financev1.UpdateYarnTxWeightGroupResponse{
		Base: successResponse("TX weight group updated successfully"),
		Data: yarnTxWeightGroupToProto(entity),
	}, nil
}

// DeleteYarnTxWeightGroup soft-deletes a group and frees its product types.
func (h *YarnTxWeightGroupHandler) DeleteYarnTxWeightGroup(ctx context.Context, req *financev1.DeleteYarnTxWeightGroupRequest) (*financev1.DeleteYarnTxWeightGroupResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpDelete, false)
		return &financev1.DeleteYarnTxWeightGroupResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GroupId)
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpDelete, false)
		return &financev1.DeleteYarnTxWeightGroupResponse{Base: invalidIDResponse(ytwgIDField)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appytwg.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpDelete, false)
		return &financev1.DeleteYarnTxWeightGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightGroupOperation(ytwgOpDelete, true)
	return &financev1.DeleteYarnTxWeightGroupResponse{
		Base: successResponse("TX weight group deleted successfully"),
	}, nil
}

// ListYarnTxWeightGroups lists groups with search, product-type filter and pagination.
func (h *YarnTxWeightGroupHandler) ListYarnTxWeightGroups(ctx context.Context, req *financev1.ListYarnTxWeightGroupsRequest) (*financev1.ListYarnTxWeightGroupsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpList, false)
		return &financev1.ListYarnTxWeightGroupsResponse{Base: baseResp}, nil
	}

	result, err := h.listHandler.Handle(ctx, appytwg.ListQuery{
		Search:        req.Search,
		ProductTypeID: req.ProductTypeId,
		Page:          int(req.Page),
		PageSize:      int(req.PageSize),
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
	})
	if err != nil {
		RecordYarnTxWeightGroupOperation(ytwgOpList, false)
		return &financev1.ListYarnTxWeightGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightGroupOperation(ytwgOpList, true)
	items := make([]*financev1.YarnTxWeightGroup, len(result.Items))
	for i, e := range result.Items {
		items[i] = yarnTxWeightGroupToProto(e)
	}
	return &financev1.ListYarnTxWeightGroupsResponse{
		Base: successResponse("TX weight groups retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ytwgInput maps the shared create/update request fields to the domain input.
func ytwgInput(code, name, description string, typeIDs []int32, rules []*financev1.YarnTxWeightRule) yarntxweightgroup.Input {
	out := make([]yarntxweightgroup.Rule, 0, len(rules))
	for _, r := range rules {
		if r == nil {
			continue
		}
		out = append(out, yarntxweightgroup.Rule{
			Grade:       yarnTxWeightGradeFromProto(r.Grade),
			Mode:        yarnTxWeightModeFromProto(r.Mode),
			Value:       r.Value,
			Description: r.Description,
		})
	}
	return yarntxweightgroup.Input{
		Code:           code,
		Name:           name,
		Description:    description,
		ProductTypeIDs: append([]int32(nil), typeIDs...),
		Rules:          out,
	}
}

// yarnTxWeightGroupToProto converts a domain group to proto.
func yarnTxWeightGroupToProto(e *yarntxweightgroup.Entity) *financev1.YarnTxWeightGroup {
	types := e.ProductTypes()
	pts := make([]*financev1.YarnTxWeightProductTypeRef, len(types))
	for i, t := range types {
		pts[i] = &financev1.YarnTxWeightProductTypeRef{Id: t.ID, Code: t.Code, Name: t.Name}
	}
	rules := e.Rules()
	prs := make([]*financev1.YarnTxWeightRule, len(rules))
	for i, r := range rules {
		prs[i] = &financev1.YarnTxWeightRule{
			Grade:       yarnTxWeightGradeToProto(r.Grade),
			Mode:        yarnTxWeightModeToProto(r.Mode),
			Value:       r.Value,
			Description: r.Description,
		}
	}
	p := &financev1.YarnTxWeightGroup{
		GroupId:      e.ID().String(),
		Code:         e.Code(),
		Name:         e.Name(),
		Description:  e.Description(),
		ProductTypes: pts,
		Rules:        prs,
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
