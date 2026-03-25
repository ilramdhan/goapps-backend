// Package rmcategory provides application layer handlers for RMCategory operations.
package rmcategory

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// CreateCommand represents the create RMCategory command.
type CreateCommand struct {
	CategoryCode string
	CategoryName string
	Description  string
	CreatedBy    string
}

// CreateHandler handles the CreateRMCategory command.
type CreateHandler struct {
	repo rmcategory.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo rmcategory.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create RMCategory command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*rmcategory.RMCategory, error) {
	// 1. Validate and create value objects
	code, err := rmcategory.NewCode(cmd.CategoryCode)
	if err != nil {
		return nil, err
	}

	// 2. Check for duplicates
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, rmcategory.ErrAlreadyExists
	}

	// 3. Create domain entity
	entity, err := rmcategory.NewRMCategory(code, cmd.CategoryName, cmd.Description, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// 4. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
