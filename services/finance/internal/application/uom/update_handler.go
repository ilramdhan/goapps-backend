// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// UpdateCommand represents the update UOM command.
type UpdateCommand struct {
	UOMID       string
	UOMName     *string
	UOMCategory *string
	Description *string
	IsActive    *bool
	UpdatedBy   string
}

// UpdateHandler handles the UpdateUOM command.
type UpdateHandler struct {
	repo uom.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo uom.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update UOM command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*uom.UOM, error) {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.UOMID)
	if err != nil {
		return nil, uom.ErrNotFound
	}

	// 2. Get existing entity
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Prepare category if provided
	var category *uom.Category
	if cmd.UOMCategory != nil {
		cat, err := uom.NewCategory(*cmd.UOMCategory)
		if err != nil {
			return nil, err
		}
		category = &cat
	}

	// 4. Update domain entity
	if err := entity.Update(cmd.UOMName, category, cmd.Description, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	// 5. Persist
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
