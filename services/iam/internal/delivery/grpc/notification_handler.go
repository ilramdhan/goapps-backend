package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appnotif "github.com/mutugading/goapps-backend/services/iam/internal/application/notification"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// NotificationHandler implements the NotificationService gRPC server.
type NotificationHandler struct {
	iamv1.UnimplementedNotificationServiceServer

	create        *appnotif.CreateHandler
	get           *appnotif.GetHandler
	list          *appnotif.ListHandler
	unreadCount   *appnotif.UnreadCountHandler
	markRead      *appnotif.MarkAsReadHandler
	markAllRead   *appnotif.MarkAllAsReadHandler
	archive       *appnotif.ArchiveHandler
	deleteHandler *appnotif.DeleteHandler
	stream        *appnotif.StreamHandler
	validation    *ValidationHelper
}

// NewNotificationHandler builds the handler.
func NewNotificationHandler(
	create *appnotif.CreateHandler,
	get *appnotif.GetHandler,
	list *appnotif.ListHandler,
	unreadCount *appnotif.UnreadCountHandler,
	markRead *appnotif.MarkAsReadHandler,
	markAllRead *appnotif.MarkAllAsReadHandler,
	archive *appnotif.ArchiveHandler,
	deleteH *appnotif.DeleteHandler,
	stream *appnotif.StreamHandler,
	v *ValidationHelper,
) *NotificationHandler {
	return &NotificationHandler{
		create: create, get: get, list: list,
		unreadCount: unreadCount, markRead: markRead, markAllRead: markAllRead,
		archive: archive, deleteHandler: deleteH, stream: stream,
		validation: v,
	}
}

// CreateNotification creates a new notification.
func (h *NotificationHandler) CreateNotification(ctx context.Context, req *iamv1.CreateNotificationRequest) (*iamv1.CreateNotificationResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateNotificationResponse{Base: baseResp}, nil
	}
	recipient, err := uuid.Parse(req.GetRecipientUserId())
	if err != nil {
		return &iamv1.CreateNotificationResponse{Base: ErrorResponse("400", "invalid recipient_user_id")}, nil //nolint:nilerr // error in body
	}
	notifType := protoToType(req.GetType())
	severity := protoToSeverity(req.GetSeverity())
	actionType := protoToActionType(req.GetActionType())

	var expiresAt *time.Time
	if s := req.GetExpiresAt(); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return &iamv1.CreateNotificationResponse{Base: ErrorResponse("400", "invalid expires_at; expected RFC3339")}, nil //nolint:nilerr // error in body
		}
		expiresAt = &t
	}

	createdBy, _ := GetUserIDFromCtx(ctx)
	// Authorization: end-users can only create notifications addressed to
	// themselves; the internal-token bypass injects "system" as user_id which
	// can target any recipient (used by service-to-service emitters).
	if createdBy != systemUser && createdBy != recipient.String() {
		return &iamv1.CreateNotificationResponse{Base: ErrorResponse("403", "forbidden: cannot create notification for another user")}, nil
	}
	if createdBy == "" {
		createdBy = systemUser
	}

	if err := validateActionPayloadJSON(req.GetActionPayload()); err != nil {
		return &iamv1.CreateNotificationResponse{Base: ErrorResponse("400", err.Error())}, nil //nolint:nilerr // error in body
	}

	n, err := h.create.Handle(ctx, appnotif.CreateCommand{
		RecipientUserID: recipient,
		Type:            notifType,
		Severity:        severity,
		Title:           req.GetTitle(),
		Body:            req.GetBody(),
		ActionType:      actionType,
		ActionPayload:   req.GetActionPayload(),
		SourceType:      req.GetSourceType(),
		SourceID:        req.GetSourceId(),
		ExpiresAt:       expiresAt,
		CreatedBy:       createdBy,
	})
	if err != nil {
		return &iamv1.CreateNotificationResponse{Base: notificationDomainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	return &iamv1.CreateNotificationResponse{Base: SuccessResponse("notification created"), Data: notificationToProto(n)}, nil
}

// notificationDomainErrorToBaseResponse maps notification domain validation
// errors to 400 BaseResponse. Other errors fall through to the generic mapper.
func notificationDomainErrorToBaseResponse(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, notification.ErrEmptyTitle),
		errors.Is(err, notification.ErrEmptyRecipient),
		errors.Is(err, notification.ErrInvalidType),
		errors.Is(err, notification.ErrInvalidSeverity),
		errors.Is(err, notification.ErrInvalidActionType),
		errors.Is(err, notification.ErrInvalidStatus):
		return ErrorResponse("400", err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// validateActionPayloadJSON returns an error when payload is non-empty and
// not parseable JSON. Empty string is allowed (e.g. action_type=NONE).
func validateActionPayloadJSON(payload string) error {
	if payload == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return errors.New("invalid action_payload: must be JSON")
	}
	return nil
}

// GetNotification fetches a single notification owned by the caller.
func (h *NotificationHandler) GetNotification(ctx context.Context, req *iamv1.GetNotificationRequest) (*iamv1.GetNotificationResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetNotificationResponse{Base: baseResp}, nil
	}
	notifID, err := uuid.Parse(req.GetNotificationId())
	if err != nil {
		return &iamv1.GetNotificationResponse{Base: ErrorResponse("400", "invalid notification_id")}, nil //nolint:nilerr // error in body
	}
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.GetNotificationResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	n, err := h.get.Handle(ctx, recipient, notifID)
	if err != nil {
		switch {
		case errors.Is(err, notification.ErrNotFound):
			return &iamv1.GetNotificationResponse{Base: NotFoundResponse("notification not found")}, nil //nolint:nilerr // error in body
		case errors.Is(err, notification.ErrForbidden):
			return &iamv1.GetNotificationResponse{Base: ErrorResponse("403", "forbidden")}, nil //nolint:nilerr // error in body
		default:
			return &iamv1.GetNotificationResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
		}
	}
	return &iamv1.GetNotificationResponse{Base: SuccessResponse("notification retrieved"), Data: notificationToProto(n)}, nil
}

// ListNotifications lists notifications for the caller.
func (h *NotificationHandler) ListNotifications(ctx context.Context, req *iamv1.ListNotificationsRequest) (*iamv1.ListNotificationsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListNotificationsResponse{Base: baseResp}, nil
	}
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.ListNotificationsResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	res, err := h.list.Handle(ctx, appnotif.ListQuery{
		RecipientUserID: recipient,
		Page:            int(req.GetPage()),
		PageSize:        int(req.GetPageSize()),
		Status:          protoToStatus(req.GetStatus()),
		Type:            protoToType(req.GetType()),
		SortOrder:       req.GetSortOrder(),
	})
	if err != nil {
		return &iamv1.ListNotificationsResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	out := make([]*iamv1.Notification, 0, len(res.Items))
	for _, n := range res.Items {
		out = append(out, notificationToProto(n))
	}
	totalPages := int64(0)
	if res.PageSize > 0 {
		totalPages = (res.TotalItems + int64(res.PageSize) - 1) / int64(res.PageSize)
	}
	return &iamv1.ListNotificationsResponse{
		Base: SuccessResponse("notifications listed"),
		Data: out,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: safeconv.IntToInt32(res.Page),
			PageSize:    safeconv.IntToInt32(res.PageSize),
			TotalItems:  res.TotalItems,
			TotalPages:  safeconv.Int64ToInt32(totalPages),
		},
	}, nil
}

// GetUnreadCount returns unread count for the caller.
func (h *NotificationHandler) GetUnreadCount(ctx context.Context, _ *iamv1.GetUnreadCountRequest) (*iamv1.GetUnreadCountResponse, error) {
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.GetUnreadCountResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	count, err := h.unreadCount.Handle(ctx, recipient)
	if err != nil {
		return &iamv1.GetUnreadCountResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	return &iamv1.GetUnreadCountResponse{Base: SuccessResponse("unread count retrieved"), UnreadCount: count}, nil
}

// MarkAsRead flips a notification to READ for the caller.
func (h *NotificationHandler) MarkAsRead(ctx context.Context, req *iamv1.MarkAsReadRequest) (*iamv1.MarkAsReadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.MarkAsReadResponse{Base: baseResp}, nil
	}
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.MarkAsReadResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	notifID, err := uuid.Parse(req.GetNotificationId())
	if err != nil {
		return &iamv1.MarkAsReadResponse{Base: ErrorResponse("400", "invalid notification_id")}, nil //nolint:nilerr // error in body
	}
	if err := h.markRead.Handle(ctx, recipient, notifID); err != nil {
		return &iamv1.MarkAsReadResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	return &iamv1.MarkAsReadResponse{Base: SuccessResponse("notification marked read")}, nil
}

// MarkAllAsRead flips all UNREAD to READ for the caller.
func (h *NotificationHandler) MarkAllAsRead(ctx context.Context, _ *iamv1.MarkAllAsReadRequest) (*iamv1.MarkAllAsReadResponse, error) {
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.MarkAllAsReadResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	affected, err := h.markAllRead.Handle(ctx, recipient)
	if err != nil {
		return &iamv1.MarkAllAsReadResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	return &iamv1.MarkAllAsReadResponse{Base: SuccessResponse("all notifications marked read"), AffectedCount: affected}, nil
}

// ArchiveNotification archives a notification.
func (h *NotificationHandler) ArchiveNotification(ctx context.Context, req *iamv1.ArchiveNotificationRequest) (*iamv1.ArchiveNotificationResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.ArchiveNotificationResponse{Base: baseResp}, nil
	}
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.ArchiveNotificationResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	notifID, err := uuid.Parse(req.GetNotificationId())
	if err != nil {
		return &iamv1.ArchiveNotificationResponse{Base: ErrorResponse("400", "invalid notification_id")}, nil //nolint:nilerr // error in body
	}
	if err := h.archive.Handle(ctx, recipient, notifID); err != nil {
		if errors.Is(err, notification.ErrNotFound) {
			return &iamv1.ArchiveNotificationResponse{Base: NotFoundResponse("notification not found")}, nil //nolint:nilerr // error in body
		}
		return &iamv1.ArchiveNotificationResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	return &iamv1.ArchiveNotificationResponse{Base: SuccessResponse("notification archived")}, nil
}

// DeleteNotification hard-deletes a notification owned by the caller.
func (h *NotificationHandler) DeleteNotification(ctx context.Context, req *iamv1.DeleteNotificationRequest) (*iamv1.DeleteNotificationResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteNotificationResponse{Base: baseResp}, nil
	}
	recipient, err := callerUserID(ctx)
	if err != nil {
		return &iamv1.DeleteNotificationResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error in body
	}
	notifID, err := uuid.Parse(req.GetNotificationId())
	if err != nil {
		return &iamv1.DeleteNotificationResponse{Base: ErrorResponse("400", "invalid notification_id")}, nil //nolint:nilerr // error in body
	}
	if err := h.deleteHandler.Handle(ctx, recipient, notifID); err != nil {
		if errors.Is(err, notification.ErrNotFound) {
			return &iamv1.DeleteNotificationResponse{Base: NotFoundResponse("notification not found")}, nil //nolint:nilerr // error in body
		}
		return &iamv1.DeleteNotificationResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error in body
	}
	return &iamv1.DeleteNotificationResponse{Base: SuccessResponse("notification deleted")}, nil
}

// StreamNotifications opens a server-streaming subscription for the caller.
func (h *NotificationHandler) StreamNotifications(req *iamv1.StreamNotificationsRequest, stream iamv1.NotificationService_StreamNotificationsServer) error {
	ctx := stream.Context()
	recipient, err := callerUserID(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, "not authenticated")
	}
	return h.stream.Handle(ctx, recipient, req.GetSince(), func(ev appnotif.StreamEvent) error {
		var notifProto *iamv1.Notification
		if ev.Notification != nil {
			notifProto = notificationToProto(ev.Notification)
		}
		return stream.Send(&iamv1.StreamNotificationsResponse{
			EventId:      ev.EventID,
			Notification: notifProto,
			IsHeartbeat:  ev.IsHeartbeat,
		})
	})
}

// callerUserID extracts and parses the authenticated user UUID from ctx.
func callerUserID(ctx context.Context) (uuid.UUID, error) {
	s, ok := GetUserIDFromCtx(ctx)
	if !ok || s == "" {
		return uuid.Nil, errors.New("not authenticated")
	}
	return uuid.Parse(s)
}

// =============================================================================
// Proto mappers
// =============================================================================

func notificationToProto(n *notification.Notification) *iamv1.Notification {
	if n == nil {
		return nil
	}
	out := &iamv1.Notification{
		NotificationId:  n.ID().String(),
		RecipientUserId: n.RecipientUserID().String(),
		Type:            typeToProto(n.Type()),
		Severity:        severityToProto(n.Severity()),
		Title:           n.Title(),
		Body:            n.Body(),
		ActionType:      actionTypeToProto(n.ActionType()),
		ActionPayload:   n.ActionPayload(),
		Status:          statusToProto(n.Status()),
		SourceType:      n.SourceType(),
		SourceId:        n.SourceID(),
		CreatedAt:       n.CreatedAt().UTC().Format(time.RFC3339Nano),
		CreatedBy:       n.CreatedBy(),
	}
	if t := n.ReadAt(); t != nil {
		out.ReadAt = t.UTC().Format(time.RFC3339Nano)
	}
	if t := n.ArchivedAt(); t != nil {
		out.ArchivedAt = t.UTC().Format(time.RFC3339Nano)
	}
	if t := n.ExpiresAt(); t != nil {
		out.ExpiresAt = t.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func protoToType(t iamv1.NotificationType) notification.Type {
	switch t {
	case iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY:
		return notification.TypeExportReady
	case iamv1.NotificationType_NOTIFICATION_TYPE_ALERT:
		return notification.TypeAlert
	case iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL:
		return notification.TypeApproval
	case iamv1.NotificationType_NOTIFICATION_TYPE_CHAT:
		return notification.TypeChat
	case iamv1.NotificationType_NOTIFICATION_TYPE_REMINDER:
		return notification.TypeReminder
	case iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM:
		return notification.TypeSystem
	case iamv1.NotificationType_NOTIFICATION_TYPE_MENTION:
		return notification.TypeMention
	case iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT:
		return notification.TypeAssignment
	case iamv1.NotificationType_NOTIFICATION_TYPE_ANNOUNCEMENT:
		return notification.TypeAnnouncement
	case iamv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func typeToProto(t notification.Type) iamv1.NotificationType {
	switch t {
	case notification.TypeExportReady:
		return iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY
	case notification.TypeAlert:
		return iamv1.NotificationType_NOTIFICATION_TYPE_ALERT
	case notification.TypeApproval:
		return iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL
	case notification.TypeChat:
		return iamv1.NotificationType_NOTIFICATION_TYPE_CHAT
	case notification.TypeReminder:
		return iamv1.NotificationType_NOTIFICATION_TYPE_REMINDER
	case notification.TypeSystem:
		return iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM
	case notification.TypeMention:
		return iamv1.NotificationType_NOTIFICATION_TYPE_MENTION
	case notification.TypeAssignment:
		return iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT
	case notification.TypeAnnouncement:
		return iamv1.NotificationType_NOTIFICATION_TYPE_ANNOUNCEMENT
	default:
		return iamv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED
	}
}

func protoToSeverity(s iamv1.NotificationSeverity) notification.Severity {
	switch s {
	case iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO:
		return notification.SeverityInfo
	case iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS:
		return notification.SeveritySuccess
	case iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING:
		return notification.SeverityWarning
	case iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_ERROR:
		return notification.SeverityError
	case iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func severityToProto(s notification.Severity) iamv1.NotificationSeverity {
	switch s {
	case notification.SeverityInfo:
		return iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case notification.SeveritySuccess:
		return iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS
	case notification.SeverityWarning:
		return iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING
	case notification.SeverityError:
		return iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_ERROR
	default:
		return iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_UNSPECIFIED
	}
}

func protoToActionType(a iamv1.NotificationActionType) notification.ActionType {
	switch a {
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NONE:
		return notification.ActionNone
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE:
		return notification.ActionNavigate
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_DOWNLOAD:
		return notification.ActionDownload
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_EXTERNAL_LINK:
		return notification.ActionExternalLink
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_APPROVE_REJECT:
		return notification.ActionApproveReject
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_ACKNOWLEDGE:
		return notification.ActionAcknowledge
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_MULTI_ACTION:
		return notification.ActionMultiAction
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_REPLY:
		return notification.ActionReply
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_SNOOZE:
		return notification.ActionSnooze
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_CUSTOM:
		return notification.ActionCustom
	case iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func actionTypeToProto(a notification.ActionType) iamv1.NotificationActionType {
	switch a {
	case notification.ActionNone:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NONE
	case notification.ActionNavigate:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE
	case notification.ActionDownload:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_DOWNLOAD
	case notification.ActionExternalLink:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_EXTERNAL_LINK
	case notification.ActionApproveReject:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_APPROVE_REJECT
	case notification.ActionAcknowledge:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_ACKNOWLEDGE
	case notification.ActionMultiAction:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_MULTI_ACTION
	case notification.ActionReply:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_REPLY
	case notification.ActionSnooze:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_SNOOZE
	case notification.ActionCustom:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_CUSTOM
	default:
		return iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_UNSPECIFIED
	}
}

func protoToStatus(s iamv1.NotificationStatus) notification.Status {
	switch s {
	case iamv1.NotificationStatus_NOTIFICATION_STATUS_UNREAD:
		return notification.StatusUnread
	case iamv1.NotificationStatus_NOTIFICATION_STATUS_READ:
		return notification.StatusRead
	case iamv1.NotificationStatus_NOTIFICATION_STATUS_ARCHIVED:
		return notification.StatusArchived
	case iamv1.NotificationStatus_NOTIFICATION_STATUS_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func statusToProto(s notification.Status) iamv1.NotificationStatus {
	switch s {
	case notification.StatusUnread:
		return iamv1.NotificationStatus_NOTIFICATION_STATUS_UNREAD
	case notification.StatusRead:
		return iamv1.NotificationStatus_NOTIFICATION_STATUS_READ
	case notification.StatusArchived:
		return iamv1.NotificationStatus_NOTIFICATION_STATUS_ARCHIVED
	default:
		return iamv1.NotificationStatus_NOTIFICATION_STATUS_UNSPECIFIED
	}
}
