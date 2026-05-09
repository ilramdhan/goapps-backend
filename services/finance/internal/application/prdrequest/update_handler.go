// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// UpdateCommand carries inputs to UpdateHandler.
type UpdateCommand struct {
	ID              uuid.UUID
	Title           string
	Description     string
	TargetSpecsJSON string
	DueDate         *time.Time
	UpdatedBy       string
}

// UpdateHandler modifies editable fields on an existing product request.
// Not allowed when the ticket is in a terminal status.
type UpdateHandler struct {
	repo domainprdrequest.Repository
}

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(repo domainprdrequest.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle fetches the request, applies the update, and persists the result.
// Returns ErrNotFound if absent, ErrInvalidTransition if in a terminal status.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domainprdrequest.Request, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := existing.Update(cmd.Title, cmd.Description, cmd.TargetSpecsJSON, cmd.DueDate, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}
