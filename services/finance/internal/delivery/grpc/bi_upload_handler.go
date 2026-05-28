package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	uploadapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/upload"
	uploaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// BIUploadHandler implements financev1.BiUploadServiceServer.
type BIUploadHandler struct {
	financev1.UnimplementedBiUploadServiceServer
	templateHandler  *uploadapp.TemplateHandler
	parseHandler     *uploadapp.ParseHandler
	commitHandler    *uploadapp.CommitHandler
	cancelHandler    *uploadapp.CancelHandler
	listHandler      *uploadapp.ListHandler
	validationHelper *ValidationHelper
}

// NewBIUploadHandler constructs the gRPC handler.
func NewBIUploadHandler(
	template *uploadapp.TemplateHandler,
	parse *uploadapp.ParseHandler,
	commit *uploadapp.CommitHandler,
	cancel *uploadapp.CancelHandler,
	list *uploadapp.ListHandler,
) (*BIUploadHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BIUploadHandler{
		templateHandler:  template,
		parseHandler:     parse,
		commitHandler:    commit,
		cancelHandler:    cancel,
		listHandler:      list,
		validationHelper: v,
	}, nil
}

// DownloadUploadTemplate returns a blank .xlsx template matching FACT_METRIC.
func (h *BIUploadHandler) DownloadUploadTemplate(_ context.Context, req *financev1.DownloadUploadTemplateRequest) (*financev1.DownloadUploadTemplateResponse, error) {
	result, err := h.templateHandler.Handle(req.GetTargetType())
	if err != nil {
		return &financev1.DownloadUploadTemplateResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.DownloadUploadTemplateResponse{
		Base:        successResponse("Template generated"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ParseUpload parses an uploaded .xlsx, validates rows, and writes a preview to staging.
func (h *BIUploadHandler) ParseUpload(ctx context.Context, req *financev1.ParseUploadRequest) (*financev1.ParseUploadResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ParseUploadResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	result, err := h.parseHandler.Handle(ctx, uploadapp.ParseCommand{
		TargetType:  req.GetTargetType(),
		FileName:    req.GetFileName(),
		FileContent: req.GetFileContent(),
		UploadedBy:  userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.ParseUploadResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.ParseUploadResponse{
		Base: successResponse("Upload parsed"),
		Data: uploadToProto(result.Upload, req.GetTargetType(), result.Errors),
	}, nil
}

// CommitUpload UPSERTs the staged rows of a previewed session into fact_metric.
func (h *BIUploadHandler) CommitUpload(ctx context.Context, req *financev1.CommitUploadRequest) (*financev1.CommitUploadResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CommitUploadResponse{Base: baseResp}, nil
	}
	session, err := h.commitHandler.Handle(ctx, uuidFromString(req.GetUploadId()))
	if err != nil {
		return &financev1.CommitUploadResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.CommitUploadResponse{
		Base: successResponse("Upload committed"),
		Data: uploadToProto(session, session.TargetType(), nil),
	}, nil
}

// CancelUpload discards a previewed session without committing.
func (h *BIUploadHandler) CancelUpload(ctx context.Context, req *financev1.CancelUploadRequest) (*financev1.CancelUploadResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CancelUploadResponse{Base: baseResp}, nil
	}
	if _, err := h.cancelHandler.Handle(ctx, uuidFromString(req.GetUploadId())); err != nil {
		return &financev1.CancelUploadResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.CancelUploadResponse{Base: successResponse("Upload cancelled")}, nil
}

// ListUploads returns paginated upload session history.
func (h *BIUploadHandler) ListUploads(ctx context.Context, req *financev1.ListUploadsRequest) (*financev1.ListUploadsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListUploadsResponse{Base: baseResp}, nil
	}
	result, err := h.listHandler.Handle(ctx, uploadapp.ListQuery{
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return &financev1.ListUploadsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.BiUpload, 0, len(result.Items))
	for _, u := range result.Items {
		items = append(items, uploadToProto(u, u.TargetType(), nil))
	}
	return &financev1.ListUploadsResponse{
		Base:       successResponse("Uploads listed"),
		Data:       items,
		Pagination: biPaginationResponse(int(req.GetPage()), int(req.GetPageSize()), int64(result.Total)),
	}, nil
}

// uploadToProto maps a domain Upload (+ optional row errors) to its proto representation.
func uploadToProto(u *uploaddomain.Upload, targetType string, errs []uploaddomain.FieldError) *financev1.BiUpload {
	if u == nil {
		return nil
	}
	out := &financev1.BiUpload{
		UploadId:      u.ID().String(),
		TargetType:    targetType,
		FileName:      u.FileName(),
		FileSize:      int64(u.FileSize()),
		Status:        u.Status(),
		TotalRows:     safeconv.IntToInt32(u.TotalRows()),
		ValidRows:     safeconv.IntToInt32(u.ValidRows()),
		InvalidRows:   safeconv.IntToInt32(u.InvalidRows()),
		CommittedRows: safeconv.IntToInt32(u.CommittedRows()),
		Errors:        rowErrorsToProto(errs),
	}
	if u.UploadedBy() != uuid.Nil {
		out.UploadedBy = u.UploadedBy().String()
	}
	if !u.UploadedAt().IsZero() {
		out.UploadedAt = timestamppb.New(u.UploadedAt())
	}
	return out
}

// rowErrorsToProto maps domain field errors to proto UploadRowError.
func rowErrorsToProto(errs []uploaddomain.FieldError) []*financev1.UploadRowError {
	if len(errs) == 0 {
		return nil
	}
	out := make([]*financev1.UploadRowError, 0, len(errs))
	for _, e := range errs {
		out = append(out, &financev1.UploadRowError{
			Row:      safeconv.IntToInt32(e.Row),
			Column:   e.Column,
			Value:    e.Value,
			Issue:    e.Issue,
			Expected: e.Expected,
		})
	}
	return out
}

// Compile-time interface check.
var _ financev1.BiUploadServiceServer = (*BIUploadHandler)(nil)
