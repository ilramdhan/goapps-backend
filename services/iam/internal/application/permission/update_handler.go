// Package permission provides application layer handlers for Permission operations.
package permission

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// UpdateCommand represents the update permission command.
type UpdateCommand struct {
	PermissionID string
	Name         *string
	Description  *string
	IsActive     *bool
	MenuID       *uuid.UUID
	UpdatedBy    string
}

// UpdateHandler handles the UpdatePermission command.
type UpdateHandler struct {
	repo role.PermissionRepository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo role.PermissionRepository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update permission command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*role.Permission, error) {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.PermissionID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 2. Get existing entity
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Update domain entity
	if err := entity.Update(cmd.Name, cmd.Description, cmd.IsActive, cmd.MenuID, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	// 4. Persist
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
