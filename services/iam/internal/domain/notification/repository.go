package notification

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ListFilter narrows results returned by Repository.ListByRecipient.
type ListFilter struct {
	Status   Status // empty = no filter
	Type     Type   // empty = no filter
	SortDesc bool   // true = newest first (default)
	Page     int    // 1-based
	PageSize int
	After    *time.Time // optional: only created_at > After (for streaming catchup)
}

// Repository is the persistence contract for Notification.
type Repository interface {
	// Create persists a new notification.
	Create(ctx context.Context, n *Notification) error

	// GetByID returns a notification by its ID.
	// Returns ErrNotFound if the row doesn't exist.
	GetByID(ctx context.Context, id uuid.UUID) (*Notification, error)

	// ListByRecipient lists notifications for a recipient with paging.
	// Returns (items, totalItems, error).
	ListByRecipient(ctx context.Context, recipientID uuid.UUID, filter ListFilter) ([]*Notification, int64, error)

	// CountUnread returns the count of UNREAD notifications for a recipient.
	CountUnread(ctx context.Context, recipientID uuid.UUID) (int64, error)

	// MarkAsRead flips a single notification to READ for the given recipient.
	// Returns ErrNotFound when the row does not exist or is owned by another user.
	MarkAsRead(ctx context.Context, recipientID, notificationID uuid.UUID, readAt time.Time) error

	// MarkAllAsRead flips all UNREAD notifications for the recipient to READ.
	// Returns the number of rows affected.
	MarkAllAsRead(ctx context.Context, recipientID uuid.UUID, readAt time.Time) (int64, error)

	// Archive flips a single notification to ARCHIVED for the recipient.
	Archive(ctx context.Context, recipientID, notificationID uuid.UUID, archivedAt time.Time) error

	// Delete hard-deletes a single notification owned by the recipient.
	Delete(ctx context.Context, recipientID, notificationID uuid.UUID) error

	// DeleteExpired hard-deletes all notifications whose expires_at < now.
	// Returns the number of rows affected. Used by a periodic cleanup job.
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}
