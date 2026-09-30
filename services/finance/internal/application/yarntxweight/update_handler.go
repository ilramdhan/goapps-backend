// Package yarntxweight provides application layer handlers for the TX Weight master.
package yarntxweight

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// UpdateCommand represents the update TX Weight rule command. Product type and
// grade are immutable.
type UpdateCommand struct {
	ID          uuid.UUID
	Mode        *yarntxweight.Mode
	Value       *float64
	Description *string
	UpdatedBy   string
}

// UpdateHandler handles UpdateYarnTxWeight.
type UpdateHandler struct {
	repo yarntxweight.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo yarntxweight.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle applies the optional changes and persists them.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*yarntxweight.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(yarntxweight.UpdateInput{
		Mode:        cmd.Mode,
		Value:       cmd.Value,
		Description: cmd.Description,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
