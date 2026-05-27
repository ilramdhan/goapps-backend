// Package employeelevel provides application layer handlers for employee level operations.
package employeelevel

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// UpdateCommand is the command for updating an employee level.
type UpdateCommand struct {
	EmployeeLevelID string
	Name            *string
	Grade           *int32
	Type            *employeelevel.Type
	Sequence        *int32
	Workflow        *employeelevel.Workflow
	IsActive        *bool
	UpdatedBy       string
}

// UpdateHandler handles UpdateEmployeeLevel commands.
type UpdateHandler struct {
	repo employeelevel.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo employeelevel.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*employeelevel.EmployeeLevel, error) {
	id, err := uuid.Parse(cmd.EmployeeLevelID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.Name, cmd.Grade, cmd.Type, cmd.Sequence, cmd.Workflow, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
