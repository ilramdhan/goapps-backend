// Package rmcategory provides application layer handlers for RMCategory operations.
package rmcategory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// UpdateCommand represents the update RMCategory command.
type UpdateCommand struct {
	RMCategoryID string
	CategoryName *string
	Description  *string
	IsActive     *bool
	UpdatedBy    string
}

// UpdateHandler handles the UpdateRMCategory command.
type UpdateHandler struct {
	repo rmcategory.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo rmcategory.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update RMCategory command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*rmcategory.RMCategory, error) {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.RMCategoryID)
	if err != nil {
		return nil, rmcategory.ErrNotFound
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
