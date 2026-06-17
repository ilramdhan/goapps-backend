// Package mbspin provides application layer handlers for MB Spin operations.
package mbspin

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// UpdateCommand represents the update MB Spin command.
type UpdateCommand struct {
	ID        uuid.UUID
	MgtName   *string
	MBCosting *string
	Denier    *float64
	Filament  *int
	Dozing    *float64
	IsActive  *bool
	UpdatedBy string
}

// UpdateHandler handles the UpdateMBSpin command.
type UpdateHandler struct {
	repo mbspin.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbspin.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB Spin command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbspin.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(mbspin.UpdateInput{
		MgtName:   cmd.MgtName,
		MBCosting: cmd.MBCosting,
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
