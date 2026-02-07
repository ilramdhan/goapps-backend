// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// GetPermissionsQuery represents the get permissions for a role query.
type GetPermissionsQuery struct {
	RoleID string
}

// GetPermissionsHandler handles the GetPermissions query.
type GetPermissionsHandler struct {
	repo role.Repository
}

// NewGetPermissionsHandler creates a new GetPermissionsHandler.
func NewGetPermissionsHandler(repo role.Repository) *GetPermissionsHandler {
	return &GetPermissionsHandler{repo: repo}
}

// Handle executes the get permissions query.
func (h *GetPermissionsHandler) Handle(ctx context.Context, query GetPermissionsQuery) ([]*role.Permission, error) {
	// 1. Parse role ID
	roleID, err := uuid.Parse(query.RoleID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 2. Verify role exists
	_, err = h.repo.GetByID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	// 3. Get permissions
	return h.repo.GetPermissions(ctx, roleID)
}
