// Package shade provides application layer handlers for the shade master (R8).
package shade

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// UpdateCommand represents the update shade command. Setting IsActive=false is
// the manual "deactivate" path — there is no separate delete command, matching
// the ppc customer precedent (deactivate, never hard-delete a synced master row).
type UpdateCommand struct {
	ID        int64
	Name      *string
	ShortName *string
	IsActive  *bool
	UpdatedBy string
}

// UpdateHandler handles the UpdateShade command.
type UpdateHandler struct {
	repo shade.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo shade.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update shade command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*shade.Shade, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(shade.UpdateParams{
		Name: cmd.Name, ShortName: cmd.ShortName, IsActive: cmd.IsActive, UpdatedBy: cmd.UpdatedBy,
	}); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
