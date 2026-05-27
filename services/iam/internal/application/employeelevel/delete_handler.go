// Package employeelevel provides application layer handlers for employee level operations.
package employeelevel

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// DeleteCommand is the command for soft-deleting an employee level.
type DeleteCommand struct {
	EmployeeLevelID string
	DeletedBy       string
}

// DeleteHandler handles DeleteEmployeeLevel commands.
type DeleteHandler struct {
	repo employeelevel.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo employeelevel.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the command.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	id, err := uuid.Parse(cmd.EmployeeLevelID)
	if err != nil {
		return shared.ErrNotFound
	}
	return h.repo.Delete(ctx, id, cmd.DeletedBy)
}
