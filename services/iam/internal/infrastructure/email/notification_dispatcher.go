package email

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

// UserEmailLookup resolves email address and display name from a user UUID.
// A nil result (empty strings, nil error) means the user is not found.
type UserEmailLookup interface {
	LookupEmail(ctx context.Context, userID uuid.UUID) (email, displayName string, err error)
}

// NotificationDispatcher implements appnotif.EmailDispatcher using the IAM SMTP service.
type NotificationDispatcher struct {
	svc    *Service
	lookup UserEmailLookup
}

// NewNotificationDispatcher wraps the SMTP Service as an EmailDispatcher.
func NewNotificationDispatcher(svc *Service, lookup UserEmailLookup) *NotificationDispatcher {
	return &NotificationDispatcher{svc: svc, lookup: lookup}
}

// Dispatch resolves the recipient's email then sends a notification email.
// Called from a background goroutine by RequestHandler — never panics.
func (d *NotificationDispatcher) Dispatch(ctx context.Context, n *notification.Notification) {
	if n == nil {
		return
	}
	email, displayName, err := d.lookup.LookupEmail(ctx, n.RecipientUserID())
	if err != nil {
		log.Warn().Err(err).
			Str("notification_id", n.ID().String()).
			Msg("NotificationDispatcher: email lookup failed (non-fatal)")
		return
	}
	if email == "" {
		log.Debug().
			Str("notification_id", n.ID().String()).
			Str("recipient_user_id", n.RecipientUserID().String()).
			Msg("NotificationDispatcher: no email for user, skip")
		return
	}
	ctaURL := d.buildCTAURL(n)
	if sendErr := d.svc.SendNotification(ctx, email, displayName, n.Title(), n.Body(), ctaURL); sendErr != nil {
		log.Warn().Err(sendErr).
			Str("notification_id", n.ID().String()).
			Str("recipient", email).
			Msg("NotificationDispatcher: send email failed (non-fatal)")
	}
}

// buildCTAURL returns a full URL for NAVIGATE-type notifications, or an empty
// string for all other action types. The path from action_payload is appended
// to the application base URL.
func (d *NotificationDispatcher) buildCTAURL(n *notification.Notification) string {
	if n.ActionType() != notification.ActionNavigate {
		return ""
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(n.ActionPayload()), &payload); err != nil || payload.Path == "" {
		return ""
	}
	base := strings.TrimRight(d.svc.AppURL(), "/")
	if base == "" {
		return ""
	}
	return base + payload.Path
}
