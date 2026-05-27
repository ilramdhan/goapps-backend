package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costnotification"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costnotification"
)

// CostNotificationHandler implements financev1.CostNotificationServiceServer.
type CostNotificationHandler struct {
	financev1.UnimplementedCostNotificationServiceServer
	listH        *app.ListHandler
	unreadH      *app.UnreadCountHandler
	markReadH    *app.MarkReadHandler
	markAllReadH *app.MarkAllReadHandler
	validation   *ValidationHelper
}

// NewCostNotificationHandler constructs the handler.
func NewCostNotificationHandler(repo domain.Repository) (*CostNotificationHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostNotificationHandler{
		listH:        app.NewListHandler(repo),
		unreadH:      app.NewUnreadCountHandler(repo),
		markReadH:    app.NewMarkReadHandler(repo),
		markAllReadH: app.NewMarkAllReadHandler(repo),
		validation:   v,
	}, nil
}

// ListMyCostNotifications returns the caller's notifications.
func (h *CostNotificationHandler) ListMyCostNotifications(ctx context.Context, req *financev1.ListMyCostNotificationsRequest) (*financev1.ListMyCostNotificationsResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.ListMyCostNotificationsResponse{Base: b}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	if userID == "" {
		return &financev1.ListMyCostNotificationsResponse{Base: ErrorResponse("401", "authentication required")}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	res, err := h.listH.Handle(ctx, app.ListQuery{
		RecipientUserID: userID, UnreadOnly: req.GetUnreadOnly(),
		Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListMyCostNotificationsResponse{Base: notifErrToBase(err)}, nil
	}
	data := make([]*financev1.CostNotification, 0, len(res.Items))
	for _, n := range res.Items {
		data = append(data, notifToProto(n))
	}
	return &financev1.ListMyCostNotificationsResponse{
		Base: successResponse("OK"), Data: data,
		Pagination:  paginationResponse(page, pageSize, res.Total),
		UnreadCount: res.UnreadCount,
	}, nil
}

// GetMyCostNotificationUnreadCount returns the bell-badge count.
func (h *CostNotificationHandler) GetMyCostNotificationUnreadCount(ctx context.Context, _ *financev1.GetMyCostNotificationUnreadCountRequest) (*financev1.GetMyCostNotificationUnreadCountResponse, error) {
	userID, _ := GetUserIDFromCtx(ctx)
	if userID == "" {
		return &financev1.GetMyCostNotificationUnreadCountResponse{Base: ErrorResponse("401", "authentication required")}, nil
	}
	n, err := h.unreadH.Handle(ctx, userID)
	if err != nil {
		return &financev1.GetMyCostNotificationUnreadCountResponse{Base: notifErrToBase(err)}, nil
	}
	return &financev1.GetMyCostNotificationUnreadCountResponse{
		Base: successResponse("OK"), UnreadCount: n,
	}, nil
}

// MarkCostNotificationRead flips a single notification to read.
func (h *CostNotificationHandler) MarkCostNotificationRead(ctx context.Context, req *financev1.MarkCostNotificationReadRequest) (*financev1.MarkCostNotificationReadResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.MarkCostNotificationReadResponse{Base: b}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	if userID == "" {
		return &financev1.MarkCostNotificationReadResponse{Base: ErrorResponse("401", "authentication required")}, nil
	}
	n, err := h.markReadH.Handle(ctx, app.MarkReadCommand{NotificationID: req.GetNotificationId(), RecipientUserID: userID})
	if err != nil {
		return &financev1.MarkCostNotificationReadResponse{Base: notifErrToBase(err)}, nil
	}
	return &financev1.MarkCostNotificationReadResponse{Base: successResponse("Marked read"), Data: notifToProto(n)}, nil
}

// MarkAllMyCostNotificationsRead flips every unread row.
func (h *CostNotificationHandler) MarkAllMyCostNotificationsRead(ctx context.Context, _ *financev1.MarkAllMyCostNotificationsReadRequest) (*financev1.MarkAllMyCostNotificationsReadResponse, error) {
	userID, _ := GetUserIDFromCtx(ctx)
	if userID == "" {
		return &financev1.MarkAllMyCostNotificationsReadResponse{Base: ErrorResponse("401", "authentication required")}, nil
	}
	updated, err := h.markAllReadH.Handle(ctx, userID)
	if err != nil {
		return &financev1.MarkAllMyCostNotificationsReadResponse{Base: notifErrToBase(err)}, nil
	}
	return &financev1.MarkAllMyCostNotificationsReadResponse{Base: successResponse("OK"), UpdatedCount: updated}, nil
}

func notifToProto(n *domain.Notification) *financev1.CostNotification {
	out := &financev1.CostNotification{
		NotificationId: n.NotificationID, RecipientUserId: n.RecipientUserID,
		TriggerType: n.TriggerType, Payload: n.Payload, IsRead: n.IsRead,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
	if n.RequestID != nil {
		out.RequestId = *n.RequestID
	}
	if t := n.EmailSentAt; t != nil {
		out.EmailSentAt = t.Format(time.RFC3339)
	}
	return out
}

func notifErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrNotRecipient):
		return ErrorResponse("403", err.Error())
	case errors.Is(err, domain.ErrInvalidTrigger):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
