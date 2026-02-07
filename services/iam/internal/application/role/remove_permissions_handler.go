// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// RemovePermissionsCommand represents the remove permissions from role command.
type RemovePermissionsCommand struct {
	RoleID        string
	PermissionIDs []string
}

// RemovePermissionsHandler handles the RemovePermissions command.
type RemovePermissionsHandler struct {
	repo role.Repository
}

// NewRemovePermissionsHandler creates a new RemovePermissionsHandler.
func NewRemovePermissionsHandler(repo role.Repository) *RemovePermissionsHandler {
	return &RemovePermissionsHandler{repo: repo}
}

// Handle executes the remove permissions command.
func (h *RemovePermissionsHandler) Handle(ctx context.Context, cmd RemovePermissionsCommand) error {
	// 1. Parse role ID
	roleID, err := uuid.Parse(cmd.RoleID)
	if err != nil {
		return shared.ErrNotFound
	}

	// 2. Verify role exists
	_, err = h.repo.GetByID(ctx, roleID)
	if err != nil {
		return err
	}

	// 3. Parse permission IDs
	permissionIDs := make([]uuid.UUID, 0, len(cmd.PermissionIDs))
	for _, idStr := range cmd.PermissionIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return shared.NewValidationError("permissionIds", "invalid permission ID: "+idStr)
		}
		permissionIDs = append(permissionIDs, id)
	}

	// 4. Remove permissions
	return h.repo.RemovePermissions(ctx, roleID, permissionIDs)
}
