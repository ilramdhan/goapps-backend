// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// UpdateCommand represents the update role command.
type UpdateCommand struct {
	RoleID      string
	Name        *string
	Description *string
	IsActive    *bool
	UpdatedBy   string
}

// UpdateHandler handles the UpdateRole command.
type UpdateHandler struct {
	repo role.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo role.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update role command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*role.Role, error) {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.RoleID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 2. Get existing entity
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Update domain entity
	if err := entity.Update(cmd.Name, cmd.Description, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	// 4. Persist
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
