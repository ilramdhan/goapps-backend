// Package uomcategory provides application layer handlers for UOM Category operations.
package uomcategory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

// UpdateCommand represents the update UOM Category command.
type UpdateCommand struct {
	UOMCategoryID string
	CategoryName  *string
	Description   *string
	IsActive      *bool
	UpdatedBy     string
}

// UpdateHandler handles the UpdateUOMCategory command.
type UpdateHandler struct {
	repo uomcategory.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo uomcategory.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update UOM Category command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*uomcategory.Category, error) {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.UOMCategoryID)
	if err != nil {
		return nil, uomcategory.ErrNotFound
	}

	// 2. Get existing entity
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Update domain entity
	if err := entity.Update(cmd.CategoryName, cmd.Description, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	// 4. Persist
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
