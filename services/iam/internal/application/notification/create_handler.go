// Package notification provides application-layer handlers for the
// notification system. Other services (finance, etc.) call CreateHandler
// via gRPC; the BFF and frontend exercise the rest.
package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

// CreateCommand carries the validated input for CreateHandler.
type CreateCommand struct {
	RecipientUserID uuid.UUID
	Type            notification.Type
	Severity        notification.Severity
	Title           string
	Body            string
	ActionType      notification.ActionType
	ActionPayload   string // JSON-encoded
	SourceType      string
	SourceID        string
	ExpiresAt       *time.Time
	CreatedBy       string
}

// CreateHandler persists a new notification and publishes it to the
// in-memory broadcaster so any open SSE subscribers receive it immediately.
type CreateHandler struct {
	repo        notification.Repository
	broadcaster *notifinfra.Broadcaster
}

// NewCreateHandler constructs the handler.
func NewCreateHandler(repo notification.Repository, b *notifinfra.Broadcaster) *CreateHandler {
	return &CreateHandler{repo: repo, broadcaster: b}
}

// Handle creates and publishes a notification.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*notification.Notification, error) {
	n, err := notification.NewNotification(
		cmd.RecipientUserID,
		cmd.Type, cmd.Severity,
		cmd.Title, cmd.Body,
		cmd.ActionType, cmd.ActionPayload,
		cmd.SourceType, cmd.SourceID, cmd.CreatedBy,
		cmd.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("build notification: %w", err)
	}
	if err := h.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("persist notification: %w", err)
	}
	if h.broadcaster != nil {
		h.broadcaster.Publish(n)
	}
	return n, nil
}
