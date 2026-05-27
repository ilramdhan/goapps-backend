// Package uomcategory provides application layer handlers for UOM Category operations.
package uomcategory

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

// CreateCommand represents the create UOM Category command.
type CreateCommand struct {
	CategoryCode string
	CategoryName string
	Description  string
	CreatedBy    string
}

// CreateHandler handles the CreateUOMCategory command.
type CreateHandler struct {
	repo uomcategory.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo uomcategory.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create UOM Category command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*uomcategory.Category, error) {
	// 1. Validate and create value objects
	code, err := uomcategory.NewCode(cmd.CategoryCode)
	if err != nil {
		return nil, err
	}

	// 2. Check for duplicates
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, uomcategory.ErrAlreadyExists
	}

	// 3. Create domain entity
	entity, err := uomcategory.NewCategory(code, cmd.CategoryName, cmd.Description, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// 4. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
