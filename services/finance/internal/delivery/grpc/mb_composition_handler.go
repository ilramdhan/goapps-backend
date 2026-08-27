// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbcomposition "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// MBCompositionHandler implements financev1.MbCompositionServiceServer.
type MBCompositionHandler struct {
	financev1.UnimplementedMbCompositionServiceServer
	createHandler       *appmbcomposition.CreateHandler
	updateHandler       *appmbcomposition.UpdateHandler
	deleteHandler       *appmbcomposition.DeleteHandler
	listHandler         *appmbcomposition.ListHandler
	listVersionsHandler *appmbcomposition.ListVersionsHandler
	validation          *ValidationHelper
}

// NewMBCompositionHandler constructs an MBCompositionHandler.
func NewMBCompositionHandler(repo mbcomposition.Repository) (*MBCompositionHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBCompositionHandler{
		createHandler:       appmbcomposition.NewCreateHandler(repo),
		updateHandler:       appmbcomposition.NewUpdateHandler(repo),
		deleteHandler:       appmbcomposition.NewDeleteHandler(repo),
		listHandler:         appmbcomposition.NewListHandler(repo),
		listVersionsHandler: appmbcomposition.NewListVersionsHandler(repo),
		validation:          v,
	}, nil
}

// CreateMbComposition creates a new MB composition line.
func (h *MBCompositionHandler) CreateMbComposition(ctx context.Context, req *financev1.CreateMbCompositionRequest) (*financev1.CreateMbCompositionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCompositionOperation("create", false)
		return &financev1.CreateMbCompositionResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmbcomposition.CreateCommand{
		MbhID:          req.MbhId,
		GroupHeadID:    req.GroupHeadId,
		CompositionPct: req.CompositionPct,
		SourceType:     req.SourceType,
		SeqNo:          req.SeqNo,
		MbRefMbhID:     req.MbRefMbhId,
		IsCarrier:      req.IsCarrier,
		CreatedBy:      getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBCompositionOperation("create", false)
		return &financev1.CreateMbCompositionResponse{Base: mbCompositionErrToBase(err)}, nil
	}

	RecordMBCompositionOperation("create", true)
	return &financev1.CreateMbCompositionResponse{
		Base: successResponse("MB composition created successfully"),
		Data: mbCompositionEntityToProto(entity),
	}, nil
}

// UpdateMbComposition updates an existing MB composition line.
func (h *MBCompositionHandler) UpdateMbComposition(ctx context.Context, req *financev1.UpdateMbCompositionRequest) (*financev1.UpdateMbCompositionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCompositionOperation("update", false)
		return &financev1.UpdateMbCompositionResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmbcomposition.UpdateCommand{
		ID:             req.MbcmId,
		GroupHeadID:    req.GroupHeadId,
		CompositionPct: req.CompositionPct,
		SourceType:     req.SourceType,
		MbRefMbhID:     req.MbRefMbhId,
		IsCarrier:      req.IsCarrier,
		UpdatedBy:      getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBCompositionOperation("update", false)
		return &financev1.UpdateMbCompositionResponse{Base: mbCompositionErrToBase(err)}, nil
	}

	RecordMBCompositionOperation("update", true)
	return &financev1.UpdateMbCompositionResponse{
		Base: successResponse("MB composition updated successfully"),
		Data: mbCompositionEntityToProto(entity),
	}, nil
}

// DeleteMbComposition soft-deletes an MB composition line.
func (h *MBCompositionHandler) DeleteMbComposition(ctx context.Context, req *financev1.DeleteMbCompositionRequest) (*financev1.DeleteMbCompositionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCompositionOperation("delete", false)
		return &financev1.DeleteMbCompositionResponse{Base: baseResp}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appmbcomposition.DeleteCommand{ID: req.MbcmId}); err != nil {
		RecordMBCompositionOperation("delete", false)
		return &financev1.DeleteMbCompositionResponse{Base: mbCompositionErrToBase(err)}, nil
	}

	RecordMBCompositionOperation("delete", true)
	return &financev1.DeleteMbCompositionResponse{Base: successResponse("MB composition deleted successfully")}, nil
}

// ListMbCompositions lists composition lines for an MB head.
func (h *MBCompositionHandler) ListMbCompositions(ctx context.Context, req *financev1.ListMbCompositionsRequest) (*financev1.ListMbCompositionsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCompositionOperation("list", false)
		return &financev1.ListMbCompositionsResponse{Base: baseResp}, nil
	}

	entities, err := h.listHandler.Handle(ctx, appmbcomposition.ListQuery{MbhID: req.MbhId})
	if err != nil {
		RecordMBCompositionOperation("list", false)
		return &financev1.ListMbCompositionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCompositionOperation("list", true)

	items := make([]*financev1.MbComposition, len(entities))
	for i, e := range entities {
		items[i] = mbCompositionEntityToProto(e)
	}

	return &financev1.ListMbCompositionsResponse{
		Base: successResponse("MB compositions retrieved successfully"),
		Data: items,
	}, nil
}

// ListMbCompositionVersions lists frozen composition version snapshot rows for an MB head.
func (h *MBCompositionHandler) ListMbCompositionVersions(ctx context.Context, req *financev1.ListMbCompositionVersionsRequest) (*financev1.ListMbCompositionVersionsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCompositionOperation("list_versions", false)
		return &financev1.ListMbCompositionVersionsResponse{Base: baseResp}, nil
	}

	rows, err := h.listVersionsHandler.Handle(ctx, appmbcomposition.ListVersionsQuery{
		MbhID:   req.MbhId,
		Version: req.Version,
	})
	if err != nil {
		RecordMBCompositionOperation("list_versions", false)
		return &financev1.ListMbCompositionVersionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCompositionOperation("list_versions", true)

	items := make([]*financev1.MbCompositionVersion, len(rows))
	for i, v := range rows {
		items[i] = mbCompositionVersionToProto(v)
	}

	return &financev1.ListMbCompositionVersionsResponse{
		Base: successResponse("MB composition versions retrieved successfully"),
		Data: items,
	}, nil
}

// mbCompositionErrToBase maps mbcomposition write-path sentinels to a BaseResponse,
// falling back to the shared domainErrorToBaseResponse for everything else.
//
// Only ErrParentNotDraft needs an explicit arm: the other composition sentinels
// already land correctly through domainErrorToBaseResponse's substring matching
// (both ErrNotFound and ErrParentHeadNotFound contain "not found" → 404), whereas
// "can only be modified while the parent mb head is in DRAFT status" matches none of
// its substrings and would otherwise be reported as a 500 server error — which is
// wrong twice over: it is the caller's precondition that failed, and a 500 tells the
// operator nothing actionable.
//
// 412 is the HTTP code this project's grpcCodeToHTTPStatus assigns to
// codes.FailedPrecondition, which is the right gRPC code here: the request is
// well-formed, the resource exists, but the parent head's state forbids the write.
func mbCompositionErrToBase(err error) *commonv1.BaseResponse {
	if errors.Is(err, mbcomposition.ErrParentNotDraft) {
		return ErrorResponse("412", err.Error())
	}
	return domainErrorToBaseResponse(err)
}

// mbCompositionEntityToProto converts a domain mbcomposition Entity to its proto representation.
func mbCompositionEntityToProto(e *mbcomposition.Entity) *financev1.MbComposition {
	return &financev1.MbComposition{
		MbcmId:         e.ID(),
		MbhId:          e.MbhID(),
		SeqNo:          e.SeqNo(),
		GroupHeadId:    e.GroupHeadID(),
		CompositionPct: e.CompositionPct(),
		SourceType:     e.SourceType(),
		MbRefMbhId:     e.MbRefMbhID(),
		IsCarrier:      e.IsCarrier(),
		LegacySysId:    e.LegacySysID(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt(),
			CreatedBy: e.CreatedBy(),
			UpdatedAt: e.UpdatedAt(),
			UpdatedBy: e.UpdatedBy(),
		},
	}
}

// mbCompositionVersionToProto converts a domain mbcomposition VersionRow read model to its proto representation.
func mbCompositionVersionToProto(v mbcomposition.VersionRow) *financev1.MbCompositionVersion {
	return &financev1.MbCompositionVersion{
		MbcvId:         v.ID,
		MbhId:          v.MbhID,
		Version:        v.Version,
		ValidatedAt:    v.ValidatedAt,
		ValidatedBy:    v.ValidatedBy,
		SeqNo:          v.SeqNo,
		GroupHeadId:    v.GroupHeadID,
		CompositionPct: v.CompositionPct,
		SourceType:     v.SourceType,
		MbRefMbhId:     v.MbRefMbhID,
		IsCarrier:      v.IsCarrier,
	}
}
