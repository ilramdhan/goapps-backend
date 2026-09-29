// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appyarntxweight "github.com/mutugading/goapps-backend/services/finance/internal/application/yarntxweight"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// YarnTxWeightHandler implements financev1.YarnTxWeightServiceServer.
type YarnTxWeightHandler struct {
	financev1.UnimplementedYarnTxWeightServiceServer
	createHandler *appyarntxweight.CreateHandler
	getHandler    *appyarntxweight.GetHandler
	listHandler   *appyarntxweight.ListHandler
	updateHandler *appyarntxweight.UpdateHandler
	deleteHandler *appyarntxweight.DeleteHandler
	validation    *ValidationHelper
}

// NewYarnTxWeightHandler constructs a YarnTxWeightHandler.
func NewYarnTxWeightHandler(repo yarntxweight.Repository) (*YarnTxWeightHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &YarnTxWeightHandler{
		createHandler: appyarntxweight.NewCreateHandler(repo),
		getHandler:    appyarntxweight.NewGetHandler(repo),
		listHandler:   appyarntxweight.NewListHandler(repo),
		updateHandler: appyarntxweight.NewUpdateHandler(repo),
		deleteHandler: appyarntxweight.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateYarnTxWeight creates a new TX Weight rule.
func (h *YarnTxWeightHandler) CreateYarnTxWeight(ctx context.Context, req *financev1.CreateYarnTxWeightRequest) (*financev1.CreateYarnTxWeightResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightOperation("create", false)
		return &financev1.CreateYarnTxWeightResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appyarntxweight.CreateCommand{
		ProductTypeID: req.ProductTypeId,
		Grade:         yarnTxWeightGradeFromProto(req.Grade),
		Mode:          yarnTxWeightModeFromProto(req.Mode),
		Value:         req.Value,
		Description:   req.Description,
		CreatedBy:     getUserFromContext(ctx),
	})
	if err != nil {
		RecordYarnTxWeightOperation("create", false)
		return &financev1.CreateYarnTxWeightResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightOperation("create", true)
	return &financev1.CreateYarnTxWeightResponse{
		Base: successResponse("TX weight rule created successfully"),
		Data: yarnTxWeightEntityToProto(entity),
	}, nil
}

// GetYarnTxWeight retrieves a TX Weight rule by ID.
func (h *YarnTxWeightHandler) GetYarnTxWeight(ctx context.Context, req *financev1.GetYarnTxWeightRequest) (*financev1.GetYarnTxWeightResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightOperation("get", false)
		return &financev1.GetYarnTxWeightResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		RecordYarnTxWeightOperation("get", false)
		return &financev1.GetYarnTxWeightResponse{Base: invalidIDResponse("id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appyarntxweight.GetQuery{ID: id})
	if err != nil {
		RecordYarnTxWeightOperation("get", false)
		return &financev1.GetYarnTxWeightResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightOperation("get", true)
	return &financev1.GetYarnTxWeightResponse{
		Base: successResponse("TX weight rule retrieved successfully"),
		Data: yarnTxWeightEntityToProto(entity),
	}, nil
}

// UpdateYarnTxWeight updates mode/value/description of a TX Weight rule.
func (h *YarnTxWeightHandler) UpdateYarnTxWeight(ctx context.Context, req *financev1.UpdateYarnTxWeightRequest) (*financev1.UpdateYarnTxWeightResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightOperation("update", false)
		return &financev1.UpdateYarnTxWeightResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		RecordYarnTxWeightOperation("update", false)
		return &financev1.UpdateYarnTxWeightResponse{Base: invalidIDResponse("id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	cmd := appyarntxweight.UpdateCommand{
		ID:          id,
		Value:       req.Value,
		Description: req.Description,
		UpdatedBy:   getUserFromContext(ctx),
	}
	if req.Mode != nil {
		m := yarnTxWeightModeFromProto(*req.Mode)
		cmd.Mode = &m
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordYarnTxWeightOperation("update", false)
		return &financev1.UpdateYarnTxWeightResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightOperation("update", true)
	return &financev1.UpdateYarnTxWeightResponse{
		Base: successResponse("TX weight rule updated successfully"),
		Data: yarnTxWeightEntityToProto(entity),
	}, nil
}

// DeleteYarnTxWeight soft-deletes a TX Weight rule.
func (h *YarnTxWeightHandler) DeleteYarnTxWeight(ctx context.Context, req *financev1.DeleteYarnTxWeightRequest) (*financev1.DeleteYarnTxWeightResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightOperation("delete", false)
		return &financev1.DeleteYarnTxWeightResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		RecordYarnTxWeightOperation("delete", false)
		return &financev1.DeleteYarnTxWeightResponse{Base: invalidIDResponse("id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appyarntxweight.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordYarnTxWeightOperation("delete", false)
		return &financev1.DeleteYarnTxWeightResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightOperation("delete", true)
	return &financev1.DeleteYarnTxWeightResponse{Base: successResponse("TX weight rule deleted successfully")}, nil
}

// ListYarnTxWeights lists TX Weight rules with search, filter, and pagination.
func (h *YarnTxWeightHandler) ListYarnTxWeights(ctx context.Context, req *financev1.ListYarnTxWeightsRequest) (*financev1.ListYarnTxWeightsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordYarnTxWeightOperation("list", false)
		return &financev1.ListYarnTxWeightsResponse{Base: baseResp}, nil
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	result, err := h.listHandler.Handle(ctx, appyarntxweight.ListQuery{
		Page:          page,
		PageSize:      pageSize,
		Search:        req.Search,
		ProductTypeID: req.ProductTypeId,
		Grade:         yarnTxWeightGradeFromProto(req.Grade),
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
	})
	if err != nil {
		RecordYarnTxWeightOperation("list", false)
		return &financev1.ListYarnTxWeightsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordYarnTxWeightOperation("list", true)

	items := make([]*financev1.YarnTxWeight, len(result.Items))
	for i, e := range result.Items {
		items[i] = yarnTxWeightEntityToProto(e)
	}

	return &financev1.ListYarnTxWeightsResponse{
		Base: successResponse("TX weight rules retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// yarnTxWeightGradeFromProto maps the proto grade enum to the domain grade.
// UNSPECIFIED maps to "" (no filter on list; rejected by the domain on create).
func yarnTxWeightGradeFromProto(g financev1.YarnTxWeightGrade) yarntxweight.Grade {
	switch g {
	case financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_AE:
		return yarntxweight.GradeAE
	case financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_A9:
		return yarntxweight.GradeA9
	case financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_A:
		return yarntxweight.GradeA
	case financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_B:
		return yarntxweight.GradeB
	case financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_C:
		return yarntxweight.GradeC
	default:
		return ""
	}
}

// yarnTxWeightGradeToProto maps the domain grade to the proto enum.
func yarnTxWeightGradeToProto(g yarntxweight.Grade) financev1.YarnTxWeightGrade {
	switch g {
	case yarntxweight.GradeAE:
		return financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_AE
	case yarntxweight.GradeA9:
		return financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_A9
	case yarntxweight.GradeA:
		return financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_A
	case yarntxweight.GradeB:
		return financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_B
	case yarntxweight.GradeC:
		return financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_C
	default:
		return financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_UNSPECIFIED
	}
}

// yarnTxWeightModeFromProto maps the proto mode enum to the domain mode.
func yarnTxWeightModeFromProto(m financev1.YarnTxWeightMode) yarntxweight.Mode {
	switch m {
	case financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_LESS_BY:
		return yarntxweight.ModeLessBy
	case financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_MULTIPLY:
		return yarntxweight.ModeMultiply
	case financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_FIXED:
		return yarntxweight.ModeFixed
	default:
		return ""
	}
}

// yarnTxWeightModeToProto maps the domain mode to the proto enum.
func yarnTxWeightModeToProto(m yarntxweight.Mode) financev1.YarnTxWeightMode {
	switch m {
	case yarntxweight.ModeLessBy:
		return financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_LESS_BY
	case yarntxweight.ModeMultiply:
		return financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_MULTIPLY
	case yarntxweight.ModeFixed:
		return financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_FIXED
	default:
		return financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_UNSPECIFIED
	}
}

// yarnTxWeightEntityToProto converts a domain TX Weight entity to proto.
func yarnTxWeightEntityToProto(e *yarntxweight.Entity) *financev1.YarnTxWeight {
	p := &financev1.YarnTxWeight{
		Id:              e.ID().String(),
		ProductTypeId:   e.ProductTypeID(),
		ProductTypeCode: e.ProductTypeCode(),
		ProductTypeName: e.ProductTypeName(),
		Grade:           yarnTxWeightGradeToProto(e.Grade()),
		Mode:            yarnTxWeightModeToProto(e.Mode()),
		Value:           e.Value(),
		Description:     e.Description(),
		OracleSysId:     e.OracleSysID(),
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
