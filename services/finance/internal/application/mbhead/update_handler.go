// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// UpdateCommand represents the update MB Head command.
type UpdateCommand struct {
	ID        uuid.UUID
	MBCosting *string
	MgtName   *string
	Denier    *float64
	Filament  *int
	Dozing    *float64
	IsActive  *bool
	UpdatedBy string
}

// UpdateHandler handles the UpdateMBHead command.
type UpdateHandler struct {
	repo mbhead.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbhead.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB Head command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(mbhead.UpdateInput{
		MBCosting: cmd.MBCosting,
		MgtName:   cmd.MgtName,
		Denier:    cmd.Denier,
		Filament:  cmd.Filament,
		Dozing:    cmd.Dozing,
		IsActive:  cmd.IsActive,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
