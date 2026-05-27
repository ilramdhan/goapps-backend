// Package employeegroup provides application layer handlers for employee group operations.
package employeegroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// UpdateCommand is the command for updating an employee group.
type UpdateCommand struct {
	EmployeeGroupID string
	Name            *string
	IsActive        *bool
	UpdatedBy       string
}

// UpdateHandler handles UpdateEmployeeGroup commands.
type UpdateHandler struct {
	repo employeegroup.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo employeegroup.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*employeegroup.EmployeeGroup, error) {
	id, err := uuid.Parse(cmd.EmployeeGroupID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.Name, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
