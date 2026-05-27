package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

// ListQuery carries paginated listing parameters scoped to a recipient.
type ListQuery struct {
	RecipientUserID uuid.UUID
	Page            int
	PageSize        int
	Status          notification.Status // empty = all
	Type            notification.Type   // empty = all
	SortOrder       string              // "", "asc", "desc"
}

// ListResult is the listing output.
type ListResult struct {
	Items      []*notification.Notification
	TotalItems int64
	Page       int
	PageSize   int
}

// ListHandler returns a recipient's paginated notifications.
type ListHandler struct {
	repo notification.Repository
}

// NewListHandler constructs the handler.
func NewListHandler(repo notification.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	if q.RecipientUserID == uuid.Nil {
		return nil, notification.ErrEmptyRecipient
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	sortDesc := q.SortOrder != "asc" // default DESC unless explicitly asc

	items, total, err := h.repo.ListByRecipient(ctx, q.RecipientUserID, notification.ListFilter{
		Status:   q.Status,
		Type:     q.Type,
		Page:     page,
		PageSize: pageSize,
		SortDesc: sortDesc,
	})
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return &ListResult{Items: items, TotalItems: total, Page: page, PageSize: pageSize}, nil
}

// GetHandler fetches a single notification owned by the caller.
type GetHandler struct {
	repo notification.Repository
}

// NewGetHandler constructs the handler.
func NewGetHandler(repo notification.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle fetches by ID and verifies ownership.
func (h *GetHandler) Handle(ctx context.Context, recipientID, notificationID uuid.UUID) (*notification.Notification, error) {
	n, err := h.repo.GetByID(ctx, notificationID)
	if err != nil {
		return nil, err
	}
	if n.RecipientUserID() != recipientID {
		return nil, notification.ErrForbidden
	}
	return n, nil
}

// UnreadCountHandler returns the recipient's unread count.
type UnreadCountHandler struct {
	repo notification.Repository
}

// NewUnreadCountHandler constructs the handler.
func NewUnreadCountHandler(repo notification.Repository) *UnreadCountHandler {
	return &UnreadCountHandler{repo: repo}
}

// Handle returns the count.
func (h *UnreadCountHandler) Handle(ctx context.Context, recipientID uuid.UUID) (int64, error) {
	if recipientID == uuid.Nil {
		return 0, notification.ErrEmptyRecipient
	}
	return h.repo.CountUnread(ctx, recipientID)
}

// MarkAsReadHandler flips a single notification to READ.
type MarkAsReadHandler struct {
	repo notification.Repository
}

// NewMarkAsReadHandler constructs the handler.
func NewMarkAsReadHandler(repo notification.Repository) *MarkAsReadHandler {
	return &MarkAsReadHandler{repo: repo}
}

// Handle marks the notification read.
func (h *MarkAsReadHandler) Handle(ctx context.Context, recipientID, notificationID uuid.UUID) error {
	return h.repo.MarkAsRead(ctx, recipientID, notificationID, time.Now().UTC())
}

// MarkAllAsReadHandler flips all UNREAD to READ for a recipient.
type MarkAllAsReadHandler struct {
	repo notification.Repository
}

// NewMarkAllAsReadHandler constructs the handler.
func NewMarkAllAsReadHandler(repo notification.Repository) *MarkAllAsReadHandler {
	return &MarkAllAsReadHandler{repo: repo}
}

// Handle marks all unread as read; returns affected count.
func (h *MarkAllAsReadHandler) Handle(ctx context.Context, recipientID uuid.UUID) (int64, error) {
	return h.repo.MarkAllAsRead(ctx, recipientID, time.Now().UTC())
}

// ArchiveHandler flips a notification to ARCHIVED.
type ArchiveHandler struct {
	repo notification.Repository
}

// NewArchiveHandler constructs the handler.
func NewArchiveHandler(repo notification.Repository) *ArchiveHandler {
	return &ArchiveHandler{repo: repo}
}

// Handle archives a notification.
func (h *ArchiveHandler) Handle(ctx context.Context, recipientID, notificationID uuid.UUID) error {
	return h.repo.Archive(ctx, recipientID, notificationID, time.Now().UTC())
}

// DeleteHandler hard-deletes a notification owned by the caller.
type DeleteHandler struct {
	repo notification.Repository
}

// NewDeleteHandler constructs the handler.
func NewDeleteHandler(repo notification.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle hard-deletes the notification.
func (h *DeleteHandler) Handle(ctx context.Context, recipientID, notificationID uuid.UUID) error {
	return h.repo.Delete(ctx, recipientID, notificationID)
}
