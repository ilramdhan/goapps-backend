// Package employeegroup provides application layer handlers for employee group operations.
package employeegroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// DeleteCommand is the command for soft-deleting an employee group.
type DeleteCommand struct {
	EmployeeGroupID string
	DeletedBy       string
}

// DeleteHandler handles DeleteEmployeeGroup commands.
type DeleteHandler struct {
	repo employeegroup.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo employeegroup.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the command.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	id, err := uuid.Parse(cmd.EmployeeGroupID)
	if err != nil {
		return shared.ErrNotFound
	}
	return h.repo.Delete(ctx, id, cmd.DeletedBy)
}
