// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// CreateCommand represents the create UOM command.
type CreateCommand struct {
	UOMCode     string
	UOMName     string
	UOMCategory string
	Description string
	CreatedBy   string
}

// CreateHandler handles the CreateUOM command.
type CreateHandler struct {
	repo uom.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo uom.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create UOM command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*uom.UOM, error) {
	// 1. Validate and create value objects
	code, err := uom.NewCode(cmd.UOMCode)
	if err != nil {
		return nil, err
	}

	category, err := uom.NewCategory(cmd.UOMCategory)
	if err != nil {
		return nil, err
	}

	// 2. Check for duplicates
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, uom.ErrAlreadyExists
	}

	// 3. Create domain entity
	entity, err := uom.NewUOM(code, cmd.UOMName, category, cmd.Description, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// 4. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
