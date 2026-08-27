// Package mbcrosssection provides application layer handlers for MB cross-section master data operations.
package mbcrosssection

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// CreateCommand represents the create MB cross-section command.
type CreateCommand struct {
	Code         string
	DisplayName  string
	Description  string
	DisplayOrder int32
	IsActive     bool
	CreatedBy    string
}

// CreateHandler handles the CreateMbCrossSection command.
type CreateHandler struct {
	repo mbcrosssection.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mbcrosssection.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB cross-section command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbcrosssection.Entity, error) {
	entity, err := mbcrosssection.NewEntity(cmd.Code, cmd.DisplayName, cmd.Description, cmd.DisplayOrder, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// NewEntity defaults isActive to true; honor an explicit inactive create.
	if !cmd.IsActive {
		entity.Deactivate()
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
