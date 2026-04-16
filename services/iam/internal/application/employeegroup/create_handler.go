// Package employeegroup provides application layer handlers for employee group operations.
package employeegroup

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// CreateCommand is the command for creating a new employee group.
type CreateCommand struct {
	Code      string
	Name      string
	CreatedBy string
}

// CreateHandler handles CreateEmployeeGroup commands.
type CreateHandler struct {
	repo employeegroup.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo employeegroup.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*employeegroup.EmployeeGroup, error) {
	code, err := employeegroup.NewCode(cmd.Code)
	if err != nil {
		return nil, err
	}

	exists, err := h.repo.ExistsByCode(ctx, code.String())
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, shared.ErrAlreadyExists
	}

	entity, err := employeegroup.NewEmployeeGroup(code, cmd.Name, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
