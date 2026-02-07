// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// CreateCommand represents the create role command.
type CreateCommand struct {
	Code        string
	Name        string
	Description string
	CreatedBy   string
}

// CreateHandler handles the CreateRole command.
type CreateHandler struct {
	repo role.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo role.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create role command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*role.Role, error) {
	// 1. Check for duplicate code
	exists, err := h.repo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, shared.ErrAlreadyExists
	}

	// 2. Create domain entity
	entity, err := role.NewRole(cmd.Code, cmd.Name, cmd.Description, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// 3. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
