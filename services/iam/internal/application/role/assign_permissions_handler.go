// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// AssignPermissionsCommand represents the assign permissions to role command.
type AssignPermissionsCommand struct {
	RoleID        string
	PermissionIDs []string
	AssignedBy    string
}

// AssignPermissionsHandler handles the AssignPermissions command.
type AssignPermissionsHandler struct {
	repo role.Repository
}

// NewAssignPermissionsHandler creates a new AssignPermissionsHandler.
func NewAssignPermissionsHandler(repo role.Repository) *AssignPermissionsHandler {
	return &AssignPermissionsHandler{repo: repo}
}

// Handle executes the assign permissions command.
func (h *AssignPermissionsHandler) Handle(ctx context.Context, cmd AssignPermissionsCommand) error {
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

	// 4. Assign permissions
	return h.repo.AssignPermissions(ctx, roleID, permissionIDs, cmd.AssignedBy)
}
