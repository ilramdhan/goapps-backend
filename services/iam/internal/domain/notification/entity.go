package notification

import (
	"time"

	"github.com/google/uuid"
)

// Notification is the aggregate root.
type Notification struct {
	id              uuid.UUID
	recipientUserID uuid.UUID
	notifType       Type
	severity        Severity
	title           string
	body            string
	actionType      ActionType
	actionPayload   string // JSON-encoded; empty when action_type=NONE
	status          Status
	readAt          *time.Time
	archivedAt      *time.Time
	expiresAt       *time.Time
	sourceType      string
	sourceID        string
	createdAt       time.Time
	createdBy       string
}

// NewNotification constructs a fresh, validated Notification ready to persist.
//
// expiresAt may be nil for never-expiring notifications.
func NewNotification(
	recipientUserID uuid.UUID,
	notifType Type,
	severity Severity,
	title, body string,
	actionType ActionType,
	actionPayload string,
	sourceType, sourceID, createdBy string,
	expiresAt *time.Time,
) (*Notification, error) {
	if recipientUserID == uuid.Nil {
		return nil, ErrEmptyRecipient
	}
	if !notifType.IsValid() {
		return nil, ErrInvalidType
	}
	if !severity.IsValid() {
		return nil, ErrInvalidSeverity
	}
	if !actionType.IsValid() {
		return nil, ErrInvalidActionType
	}
	if title == "" {
		return nil, ErrEmptyTitle
	}
	if createdBy == "" {
		createdBy = "system"
	}
	return &Notification{
		id:              uuid.New(),
		recipientUserID: recipientUserID,
		notifType:       notifType,
		severity:        severity,
		title:           title,
		body:            body,
		actionType:      actionType,
		actionPayload:   actionPayload,
		status:          StatusUnread,
		expiresAt:       expiresAt,
		sourceType:      sourceType,
		sourceID:        sourceID,
		createdAt:       time.Now().UTC(),
		createdBy:       createdBy,
	}, nil
}

// Reconstruct rebuilds a Notification from persistence (no validation).
func Reconstruct(
	id, recipientUserID uuid.UUID,
	notifType Type, severity Severity,
	title, body string,
	actionType ActionType, actionPayload string,
	status Status,
	readAt, archivedAt, expiresAt *time.Time,
	sourceType, sourceID string,
	createdAt time.Time, createdBy string,
) *Notification {
	return &Notification{
		id:              id,
		recipientUserID: recipientUserID,
		notifType:       notifType,
		severity:        severity,
		title:           title,
		body:            body,
		actionType:      actionType,
		actionPayload:   actionPayload,
		status:          status,
		readAt:          readAt,
		archivedAt:      archivedAt,
		expiresAt:       expiresAt,
		sourceType:      sourceType,
		sourceID:        sourceID,
		createdAt:       createdAt,
		createdBy:       createdBy,
	}
}

// MarkAsRead transitions the notification to READ if it was UNREAD. No-op
// otherwise (idempotent).
func (n *Notification) MarkAsRead() {
	if n.status != StatusUnread {
		return
	}
	now := time.Now().UTC()
	n.status = StatusRead
	n.readAt = &now
}

// Archive transitions the notification to ARCHIVED. Returns ErrAlreadyArchived
// if already archived.
func (n *Notification) Archive() error {
	if n.status == StatusArchived {
		return ErrAlreadyArchived
	}
	now := time.Now().UTC()
	n.status = StatusArchived
	n.archivedAt = &now
	if n.readAt == nil {
		// Archiving an unread notification implicitly marks it read.
		n.readAt = &now
	}
	return nil
}

// IsExpired reports whether the notification has passed its expires_at.
func (n *Notification) IsExpired(at time.Time) bool {
	if n.expiresAt == nil {
		return false
	}
	return at.After(*n.expiresAt)
}

// Getters.

// ID returns the notification's UUID.
func (n *Notification) ID() uuid.UUID { return n.id }

// RecipientUserID returns the target user UUID.
func (n *Notification) RecipientUserID() uuid.UUID { return n.recipientUserID }

// Type returns the notification type.
func (n *Notification) Type() Type { return n.notifType }

// Severity returns the severity.
func (n *Notification) Severity() Severity { return n.severity }

// Title returns the title.
func (n *Notification) Title() string { return n.title }

// Body returns the body.
func (n *Notification) Body() string { return n.body }

// ActionType returns the action type.
func (n *Notification) ActionType() ActionType { return n.actionType }

// ActionPayload returns the JSON-encoded action payload string.
func (n *Notification) ActionPayload() string { return n.actionPayload }

// Status returns the current status.
func (n *Notification) Status() Status { return n.status }

// ReadAt returns the read timestamp or nil.
func (n *Notification) ReadAt() *time.Time { return n.readAt }

// ArchivedAt returns the archive timestamp or nil.
func (n *Notification) ArchivedAt() *time.Time { return n.archivedAt }

// ExpiresAt returns the expiry or nil.
func (n *Notification) ExpiresAt() *time.Time { return n.expiresAt }

// SourceType returns the emitting feature identifier.
func (n *Notification) SourceType() string { return n.sourceType }

// SourceID returns the emitter's correlation id.
func (n *Notification) SourceID() string { return n.sourceID }

// CreatedAt returns the creation timestamp.
func (n *Notification) CreatedAt() time.Time { return n.createdAt }

// CreatedBy returns the creator identifier.
func (n *Notification) CreatedBy() string { return n.createdBy }
