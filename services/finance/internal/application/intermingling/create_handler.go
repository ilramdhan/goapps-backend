// Package intermingling provides application layer handlers for Intermingling operations.
package intermingling

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// CreateCommand represents the create Intermingling command.
type CreateCommand struct {
	Code      string
	Name      string
	CostPerKg float64
	Notes     string
	CreatedBy string
}

// CreateHandler handles the CreateIntermingling command.
type CreateHandler struct {
	repo intermingling.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo intermingling.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create Intermingling command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*intermingling.Entity, error) {
	exists, err := h.repo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, intermingling.ErrAlreadyExists
	}

	entity, err := intermingling.New(cmd.Code, cmd.Name, cmd.CostPerKg, cmd.Notes, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
