package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costattachment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costattachment"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// CostAttachmentHandler implements financev1.CostAttachmentServiceServer.
type CostAttachmentHandler struct {
	financev1.UnimplementedCostAttachmentServiceServer
	uploadHandler        *app.UploadHandler
	listByRequestHandler *app.ListByRequestHandler
	listByCommentHandler *app.ListByCommentHandler
	downloadURLHandler   *app.DownloadURLHandler
	deleteHandler        *app.DeleteHandler
	validation           *ValidationHelper
	hasStorage           bool
}

// NewCostAttachmentHandler constructs the handler. svc may be nil — uploads/downloads
// will then return a 503 BaseResponse instead of panicking.
func NewCostAttachmentHandler(repo domain.Repository, svc storage.Service) (*CostAttachmentHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostAttachmentHandler{
		uploadHandler:        app.NewUploadHandler(repo, svc),
		listByRequestHandler: app.NewListByRequestHandler(repo),
		listByCommentHandler: app.NewListByCommentHandler(repo),
		downloadURLHandler:   app.NewDownloadURLHandler(repo, svc),
		deleteHandler:        app.NewDeleteHandler(repo, svc),
		validation:           v,
		hasStorage:           svc != nil,
	}, nil
}

// UploadCostAttachment uploads a file to MinIO and persists the metadata.
func (h *CostAttachmentHandler) UploadCostAttachment(ctx context.Context, req *financev1.UploadCostAttachmentRequest) (*financev1.UploadCostAttachmentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UploadCostAttachmentResponse{Base: baseResp}, nil
	}
	if !h.hasStorage {
		return &financev1.UploadCostAttachmentResponse{Base: ErrorResponse("503", "storage unavailable")}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	a, err := h.uploadHandler.Handle(ctx, app.UploadCommand{
		RequestID:   req.GetRequestId(),
		CommentID:   req.GetCommentId(),
		Filename:    req.GetFilename(),
		MimeType:    req.GetMimeType(),
		FileContent: req.GetFileContent(),
		UploadedBy:  actor,
	})
	if err != nil {
		return &financev1.UploadCostAttachmentResponse{Base: attachmentErrToBase(err)}, nil
	}
	return &financev1.UploadCostAttachmentResponse{Base: successResponse("Uploaded"), Data: attachmentToProto(a)}, nil
}

// ListCostAttachmentsByRequest returns request-level attachments.
func (h *CostAttachmentHandler) ListCostAttachmentsByRequest(ctx context.Context, req *financev1.ListCostAttachmentsByRequestRequest) (*financev1.ListCostAttachmentsByRequestResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostAttachmentsByRequestResponse{Base: baseResp}, nil
	}
	items, err := h.listByRequestHandler.Handle(ctx, req.GetRequestId())
	if err != nil {
		return &financev1.ListCostAttachmentsByRequestResponse{Base: attachmentErrToBase(err)}, nil
	}
	data := make([]*financev1.CostAttachment, 0, len(items))
	for _, a := range items {
		data = append(data, attachmentToProto(a))
	}
	return &financev1.ListCostAttachmentsByRequestResponse{Base: successResponse("OK"), Data: data}, nil
}

// ListCostAttachmentsByComment returns comment-level attachments.
func (h *CostAttachmentHandler) ListCostAttachmentsByComment(ctx context.Context, req *financev1.ListCostAttachmentsByCommentRequest) (*financev1.ListCostAttachmentsByCommentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostAttachmentsByCommentResponse{Base: baseResp}, nil
	}
	items, err := h.listByCommentHandler.Handle(ctx, req.GetCommentId())
	if err != nil {
		return &financev1.ListCostAttachmentsByCommentResponse{Base: attachmentErrToBase(err)}, nil
	}
	data := make([]*financev1.CostAttachment, 0, len(items))
	for _, a := range items {
		data = append(data, attachmentToProto(a))
	}
	return &financev1.ListCostAttachmentsByCommentResponse{Base: successResponse("OK"), Data: data}, nil
}

// GetCostAttachmentDownloadURL returns a presigned URL.
func (h *CostAttachmentHandler) GetCostAttachmentDownloadURL(ctx context.Context, req *financev1.GetCostAttachmentDownloadURLRequest) (*financev1.GetCostAttachmentDownloadURLResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostAttachmentDownloadURLResponse{Base: baseResp}, nil
	}
	if !h.hasStorage {
		return &financev1.GetCostAttachmentDownloadURLResponse{Base: ErrorResponse("503", "storage unavailable")}, nil
	}
	res, err := h.downloadURLHandler.Handle(ctx, req.GetAttachmentId())
	if err != nil {
		return &financev1.GetCostAttachmentDownloadURLResponse{Base: attachmentErrToBase(err)}, nil
	}
	return &financev1.GetCostAttachmentDownloadURLResponse{
		Base:         successResponse("OK"),
		Url:          res.URL,
		ValidSeconds: res.ValidSeconds,
	}, nil
}

// DeleteCostAttachment removes the metadata + storage object.
func (h *CostAttachmentHandler) DeleteCostAttachment(ctx context.Context, req *financev1.DeleteCostAttachmentRequest) (*financev1.DeleteCostAttachmentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteCostAttachmentResponse{Base: baseResp}, nil
	}
	if err := h.deleteHandler.Handle(ctx, req.GetAttachmentId()); err != nil {
		return &financev1.DeleteCostAttachmentResponse{Base: attachmentErrToBase(err)}, nil
	}
	return &financev1.DeleteCostAttachmentResponse{Base: successResponse("Deleted")}, nil
}

// =============================================================================
// mappers
// =============================================================================

func attachmentToProto(a *domain.Attachment) *financev1.CostAttachment {
	out := &financev1.CostAttachment{
		AttachmentId: a.AttachmentID,
		Filename:     a.Filename,
		MimeType:     a.MimeType,
		SizeBytes:    a.SizeBytes,
		StorageKey:   a.StorageKey,
		UploadedBy:   a.UploadedBy,
		UploadedAt:   a.UploadedAt.Format(time.RFC3339),
	}
	if a.RequestID != nil {
		out.RequestId = *a.RequestID
	}
	if a.CommentID != nil {
		out.CommentId = *a.CommentID
	}
	return out
}

func attachmentErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrOwnerXor),
		errors.Is(err, domain.ErrInvalidFile):
		return ErrorResponse("400", err.Error())
	case errors.Is(err, app.ErrStorageRequired):
		return ErrorResponse("503", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
