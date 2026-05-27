// Package costnotification holds notification read + emit use cases.
package costnotification

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costnotification"
)

// ListQuery input.
type ListQuery struct {
	RecipientUserID string
	UnreadOnly      bool
	Page            int
	PageSize        int
}

// ListResult bundles items + total + unread count for the bell badge.
type ListResult struct {
	Items       []*domain.Notification
	Total       int64
	UnreadCount int32
}

// ListHandler returns my notifications + unread count in one shot.
type ListHandler struct{ repo domain.Repository }

// NewListHandler constructs.
func NewListHandler(r domain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle returns the paged list + unread count.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, domain.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	unread, err := h.repo.UnreadCount(ctx, q.RecipientUserID)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, UnreadCount: unread}, nil
}

// UnreadCountHandler returns just the unread count (cheaper for the bell badge).
type UnreadCountHandler struct{ repo domain.Repository }

// NewUnreadCountHandler constructs.
func NewUnreadCountHandler(r domain.Repository) *UnreadCountHandler {
	return &UnreadCountHandler{repo: r}
}

// Handle returns the count.
func (h *UnreadCountHandler) Handle(ctx context.Context, recipientUserID string) (int32, error) {
	return h.repo.UnreadCount(ctx, recipientUserID)
}

// MarkReadCommand input.
type MarkReadCommand struct {
	NotificationID  int64
	RecipientUserID string
}

// MarkReadHandler flips a single notification to read.
type MarkReadHandler struct{ repo domain.Repository }

// NewMarkReadHandler constructs.
func NewMarkReadHandler(r domain.Repository) *MarkReadHandler { return &MarkReadHandler{repo: r} }

// Handle marks the notification read after verifying the recipient.
func (h *MarkReadHandler) Handle(ctx context.Context, cmd MarkReadCommand) (*domain.Notification, error) {
	n, err := h.repo.GetByID(ctx, cmd.NotificationID)
	if err != nil {
		return nil, err
	}
	if err := n.MarkRead(cmd.RecipientUserID); err != nil {
		return nil, err
	}
	if err := h.repo.MarkRead(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

// MarkAllReadHandler flips every unread row for the recipient.
type MarkAllReadHandler struct{ repo domain.Repository }

// NewMarkAllReadHandler constructs.
func NewMarkAllReadHandler(r domain.Repository) *MarkAllReadHandler {
	return &MarkAllReadHandler{repo: r}
}

// Handle returns the number of rows updated.
func (h *MarkAllReadHandler) Handle(ctx context.Context, recipientUserID string) (int32, error) {
	return h.repo.MarkAllRead(ctx, recipientUserID)
}

// Emitter is used by business handlers to fire notifications.
type Emitter struct{ repo domain.Repository }

// NewEmitter constructs.
func NewEmitter(r domain.Repository) *Emitter { return &Emitter{repo: r} }

// Emit persists one notification.
func (e *Emitter) Emit(ctx context.Context, in domain.NewInput) (*domain.Notification, error) {
	n, err := domain.New(in)
	if err != nil {
		return nil, err
	}
	if err := e.repo.Emit(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}
