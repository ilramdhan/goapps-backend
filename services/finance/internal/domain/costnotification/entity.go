// Package costnotification is the cost_notification domain (PRD Phase A §7.1.12 + §7.1.13).
// In-app notifications with optional email-sent timestamp; preferences live in CNP_
// (separate aggregate, schema-only for now — read-update API will come with the
// dispatcher worker in a later iteration).
package costnotification

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Sentinel errors.
var (
	// ErrNotFound when a notification is missing.
	ErrNotFound = errors.New("cost notification not found")
	// ErrInvalidTrigger when trigger_type is outside the whitelist.
	ErrInvalidTrigger = errors.New("invalid trigger_type")
	// ErrNotRecipient when a non-recipient tries to mutate (mark read, etc).
	ErrNotRecipient = errors.New("only the recipient may mutate this notification")
)

// Trigger types mirror the DB CHECK constraint.
const (
	TriggerStatusChange    = "STATUS_CHANGE"
	TriggerMention         = "MENTION"
	TriggerAssigned        = "ASSIGNED"
	TriggerFeasibility     = "FEASIBILITY"
	TriggerCommentAdded    = "COMMENT_ADDED"
	TriggerRoutingPromoted = "ROUTING_PROMOTED"
	TriggerRequestRejected = "REQUEST_REJECTED"
	TriggerRequestClosed   = "REQUEST_CLOSED"
)

var allowedTriggers = map[string]struct{}{
	TriggerStatusChange: {}, TriggerMention: {}, TriggerAssigned: {},
	TriggerFeasibility: {}, TriggerCommentAdded: {}, TriggerRoutingPromoted: {},
	TriggerRequestRejected: {}, TriggerRequestClosed: {},
}

// Notification is an in-app notification row.
type Notification struct {
	NotificationID  int64
	RecipientUserID string
	TriggerType     string
	RequestID       *int64
	Payload         string // JSON-encoded
	IsRead          bool
	EmailSentAt     *time.Time
	CreatedAt       time.Time
}

// NewInput is the create-time payload (Emit).
type NewInput struct {
	RecipientUserID string
	TriggerType     string
	RequestID       int64 // 0 if not tied to a request
	Payload         string
}

// New constructs a Notification (validates trigger_type).
func New(in NewInput) (*Notification, error) {
	if _, ok := allowedTriggers[in.TriggerType]; !ok {
		return nil, ErrInvalidTrigger
	}
	if strings.TrimSpace(in.RecipientUserID) == "" {
		return nil, ErrNotRecipient // reuse for missing recipient
	}
	n := &Notification{
		RecipientUserID: in.RecipientUserID,
		TriggerType:     in.TriggerType,
		Payload:         in.Payload,
		CreatedAt:       time.Now().UTC(),
	}
	if in.RequestID > 0 {
		v := in.RequestID
		n.RequestID = &v
	}
	if strings.TrimSpace(n.Payload) == "" {
		n.Payload = "{}"
	}
	return n, nil
}

// MarkRead flips is_read=true; only the recipient may call this.
func (n *Notification) MarkRead(userID string) error {
	if n.RecipientUserID != userID {
		return ErrNotRecipient
	}
	n.IsRead = true
	return nil
}

// Filter for ListByRecipient.
type Filter struct {
	RecipientUserID string
	UnreadOnly      bool
	Page            int
	PageSize        int
}

// Repository persists notifications.
type Repository interface {
	Emit(ctx context.Context, n *Notification) error
	GetByID(ctx context.Context, id int64) (*Notification, error)
	List(ctx context.Context, f Filter) (items []*Notification, total int64, err error)
	UnreadCount(ctx context.Context, recipientUserID string) (int32, error)
	MarkRead(ctx context.Context, n *Notification) error
	// MarkAllRead returns the number of rows updated.
	MarkAllRead(ctx context.Context, recipientUserID string) (int32, error)
}
