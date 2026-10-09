package superbacostsp

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// UpdateCommand is the update command (nil fields are left unchanged).
type UpdateCommand struct {
	ID         string
	ShadeCode  *string
	ColourName *string
	OldValue   *float64
	NewValue   *float64
	IsActive   *bool
	UpdatedBy  string
}

// UpdateHandler handles UpdateSuperbaCostSp.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler creates an UpdateHandler.
func NewUpdateHandler(repo domain.Repository) *UpdateHandler { return &UpdateHandler{repo: repo} }

// Handle executes the command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.Entry, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(domain.UpdateParams{
		ShadeCode: cmd.ShadeCode, ColourName: cmd.ColourName, OldValue: cmd.OldValue,
		NewValue: cmd.NewValue, IsActive: cmd.IsActive, UpdatedBy: cmd.UpdatedBy,
	}); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
