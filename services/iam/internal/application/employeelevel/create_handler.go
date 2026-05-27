// Package employeelevel provides application layer handlers for employee level operations.
package employeelevel

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// CreateCommand is the command for creating a new employee level.
type CreateCommand struct {
	Code      string
	Name      string
	Grade     int32
	Type      employeelevel.Type
	Sequence  int32
	Workflow  employeelevel.Workflow
	CreatedBy string
}

// CreateHandler handles CreateEmployeeLevel commands.
type CreateHandler struct {
	repo employeelevel.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo employeelevel.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*employeelevel.EmployeeLevel, error) {
	code, err := employeelevel.NewCode(cmd.Code)
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

	entity, err := employeelevel.NewEmployeeLevel(code, cmd.Name, cmd.Grade, cmd.Type, cmd.Sequence, cmd.Workflow, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
