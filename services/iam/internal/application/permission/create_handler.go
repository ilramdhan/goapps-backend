// Package permission provides application layer handlers for Permission operations.
package permission

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// CreateCommand represents the create permission command.
type CreateCommand struct {
	Code        string
	Name        string
	Description string
	ServiceName string
	ModuleName  string
	ActionType  string
	CreatedBy   string
	MenuID      *uuid.UUID
}

// CreateHandler handles the CreatePermission command.
type CreateHandler struct {
	repo role.PermissionRepository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo role.PermissionRepository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create permission command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*role.Permission, error) {
	// 1. Check for duplicate code
	exists, err := h.repo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, shared.ErrAlreadyExists
	}

	// 2. Create domain entity
	entity, err := role.NewPermission(cmd.Code, cmd.Name, cmd.Description, cmd.ServiceName, cmd.ModuleName, cmd.ActionType, cmd.CreatedBy, cmd.MenuID)
	if err != nil {
		return nil, err
	}

	// 3. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
