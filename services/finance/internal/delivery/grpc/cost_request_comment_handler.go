package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costrequestcomment"
	cprdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequestcomment"
)

// CostRequestCommentHandler implements financev1.CostRequestCommentServiceServer.
type CostRequestCommentHandler struct {
	financev1.UnimplementedCostRequestCommentServiceServer
	createHandler          *app.CreateHandler
	updateHandler          *app.UpdateHandler
	hideHandler            *app.HideHandler
	unhideHandler          *app.UnhideHandler
	deleteHandler          *app.DeleteHandler
	listByRequestHandler   *app.ListByRequestHandler
	listEditHistoryHandler *app.ListEditHistoryHandler
	validation             *ValidationHelper
}

// WithCPRNotifier wires CPR notification support into the create-comment use case.
// Both arguments must be non-nil.
func (h *CostRequestCommentHandler) WithCPRNotifier(repo cprdomain.Repository, notifier cprapp.CPRNotifier) *CostRequestCommentHandler {
	h.createHandler.WithCPRNotifier(repo, notifier)
	return h
}

// NewCostRequestCommentHandler constructs the handler.
func NewCostRequestCommentHandler(repo domain.Repository) (*CostRequestCommentHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostRequestCommentHandler{
		createHandler:          app.NewCreateHandler(repo),
		updateHandler:          app.NewUpdateHandler(repo),
		hideHandler:            app.NewHideHandler(repo),
		unhideHandler:          app.NewUnhideHandler(repo),
		deleteHandler:          app.NewDeleteHandler(repo),
		listByRequestHandler:   app.NewListByRequestHandler(repo),
		listEditHistoryHandler: app.NewListEditHistoryHandler(repo),
		validation:             v,
	}, nil
}

// CreateCostRequestComment creates a comment.
func (h *CostRequestCommentHandler) CreateCostRequestComment(ctx context.Context, req *financev1.CreateCostRequestCommentRequest) (*financev1.CreateCostRequestCommentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateCostRequestCommentResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	authorName, _ := GetUsernameFromCtx(ctx)
	c, err := h.createHandler.Handle(ctx, app.CreateCommand{
		RequestID:        req.GetRequestId(),
		ParentCommentID:  req.GetParentCommentId(),
		AuthorUserID:     actor,
		AuthorName:       authorName,
		BodyRichtext:     req.GetBodyRichtext(),
		BodyPlaintext:    req.GetBodyPlaintext(),
		MentionedUserIDs: req.GetMentionedUserIds(),
	})
	if err != nil {
		return &financev1.CreateCostRequestCommentResponse{Base: commentErrToBase(err)}, nil
	}
	return &financev1.CreateCostRequestCommentResponse{Base: successResponse("Comment posted"), Data: commentToProto(c)}, nil
}

// UpdateCostRequestComment edits a comment.
func (h *CostRequestCommentHandler) UpdateCostRequestComment(ctx context.Context, req *financev1.UpdateCostRequestCommentRequest) (*financev1.UpdateCostRequestCommentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostRequestCommentResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	c, err := h.updateHandler.Handle(ctx, app.UpdateCommand{
		CommentID:        req.GetCommentId(),
		EditorUserID:     actor,
		BodyRichtext:     req.GetBodyRichtext(),
		BodyPlaintext:    req.GetBodyPlaintext(),
		MentionedUserIDs: req.GetMentionedUserIds(),
	})
	if err != nil {
		return &financev1.UpdateCostRequestCommentResponse{Base: commentErrToBase(err)}, nil
	}
	return &financev1.UpdateCostRequestCommentResponse{Base: successResponse("Comment edited"), Data: commentToProto(c)}, nil
}

// HideCostRequestComment hides a comment.
func (h *CostRequestCommentHandler) HideCostRequestComment(ctx context.Context, req *financev1.HideCostRequestCommentRequest) (*financev1.HideCostRequestCommentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.HideCostRequestCommentResponse{Base: baseResp}, nil
	}
	c, err := h.hideHandler.Handle(ctx, app.HideCommand{
		CommentID:    req.GetCommentId(),
		HiddenReason: req.GetHiddenReason(),
	})
	if err != nil {
		return &financev1.HideCostRequestCommentResponse{Base: commentErrToBase(err)}, nil
	}
	return &financev1.HideCostRequestCommentResponse{Base: successResponse("Comment hidden"), Data: commentToProto(c)}, nil
}

// UnhideCostRequestComment unhides a comment.
func (h *CostRequestCommentHandler) UnhideCostRequestComment(ctx context.Context, req *financev1.UnhideCostRequestCommentRequest) (*financev1.UnhideCostRequestCommentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UnhideCostRequestCommentResponse{Base: baseResp}, nil
	}
	c, err := h.unhideHandler.Handle(ctx, app.UnhideCommand{CommentID: req.GetCommentId()})
	if err != nil {
		return &financev1.UnhideCostRequestCommentResponse{Base: commentErrToBase(err)}, nil
	}
	return &financev1.UnhideCostRequestCommentResponse{Base: successResponse("Comment unhidden"), Data: commentToProto(c)}, nil
}

// DeleteCostRequestComment removes a comment.
func (h *CostRequestCommentHandler) DeleteCostRequestComment(ctx context.Context, req *financev1.DeleteCostRequestCommentRequest) (*financev1.DeleteCostRequestCommentResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteCostRequestCommentResponse{Base: baseResp}, nil
	}
	if err := h.deleteHandler.Handle(ctx, app.DeleteCommand{CommentID: req.GetCommentId()}); err != nil {
		return &financev1.DeleteCostRequestCommentResponse{Base: commentErrToBase(err)}, nil
	}
	return &financev1.DeleteCostRequestCommentResponse{Base: successResponse("Comment deleted")}, nil
}

// ListCostRequestComments returns the thread.
func (h *CostRequestCommentHandler) ListCostRequestComments(ctx context.Context, req *financev1.ListCostRequestCommentsRequest) (*financev1.ListCostRequestCommentsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostRequestCommentsResponse{Base: baseResp}, nil
	}
	comments, err := h.listByRequestHandler.Handle(ctx, app.ListByRequestQuery{
		RequestID:     req.GetRequestId(),
		IncludeHidden: req.GetIncludeHidden(),
	})
	if err != nil {
		return &financev1.ListCostRequestCommentsResponse{Base: commentErrToBase(err)}, nil
	}
	data := make([]*financev1.CostRequestComment, 0, len(comments))
	for _, c := range comments {
		data = append(data, commentToProto(c))
	}
	return &financev1.ListCostRequestCommentsResponse{Base: successResponse("OK"), Data: data}, nil
}

// ListCostCommentEditHistory returns CCEH_ rows.
func (h *CostRequestCommentHandler) ListCostCommentEditHistory(ctx context.Context, req *financev1.ListCostCommentEditHistoryRequest) (*financev1.ListCostCommentEditHistoryResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostCommentEditHistoryResponse{Base: baseResp}, nil
	}
	entries, err := h.listEditHistoryHandler.Handle(ctx, app.ListEditHistoryQuery{CommentID: req.GetCommentId()})
	if err != nil {
		return &financev1.ListCostCommentEditHistoryResponse{Base: commentErrToBase(err)}, nil
	}
	data := make([]*financev1.CostCommentEditHistory, 0, len(entries))
	for _, e := range entries {
		data = append(data, &financev1.CostCommentEditHistory{
			EditId:        e.EditID,
			CommentId:     e.CommentID,
			BodyRichtext:  e.BodyRichtext,
			BodyPlaintext: e.BodyPlaintext,
			EditedBy:      e.EditedBy,
			EditedAt:      e.EditedAt.Format(time.RFC3339),
		})
	}
	return &financev1.ListCostCommentEditHistoryResponse{Base: successResponse("OK"), Data: data}, nil
}

// =============================================================================
// mappers
// =============================================================================

func commentToProto(c *domain.Comment) *financev1.CostRequestComment {
	out := &financev1.CostRequestComment{
		CommentId:        c.CommentID(),
		RequestId:        c.RequestID(),
		AuthorUserId:     c.AuthorUserID(),
		BodyRichtext:     c.BodyRichtext(),
		BodyPlaintext:    c.BodyPlaintext(),
		IsEdited:         c.IsEdited(),
		IsHidden:         c.IsHidden(),
		HiddenReason:     c.HiddenReason(),
		CreatedAt:        c.CreatedAt().Format(time.RFC3339),
		UpdatedAt:        c.UpdatedAt().Format(time.RFC3339),
		MentionedUserIds: c.MentionedUserIDs(),
	}
	if p := c.ParentCommentID(); p != nil {
		out.ParentCommentId = *p
	}
	return out
}

func commentErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrNotAuthor):
		return ErrorResponse("403", err.Error())
	case errors.Is(err, domain.ErrInvalidBody),
		errors.Is(err, domain.ErrHiddenReasonRequired):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
