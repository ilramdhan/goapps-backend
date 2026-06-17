// Package intermingling provides application layer handlers for Intermingling operations.
package intermingling

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// UpdateCommand represents the update Intermingling command.
type UpdateCommand struct {
	ID        uuid.UUID
	Name      *string
	CostPerKg *float64
	Notes     *string
	IsActive  *bool
	UpdatedBy string
}

// UpdateHandler handles the UpdateIntermingling command.
type UpdateHandler struct {
	repo intermingling.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo intermingling.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Intermingling command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*intermingling.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(intermingling.UpdateInput{
		Name:      cmd.Name,
		CostPerKg: cmd.CostPerKg,
		Notes:     cmd.Notes,
		IsActive:  cmd.IsActive,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
