// Package productgrade provides application layer handlers for Product Grade operations.
package productgrade

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// CreateCommand represents the create Product Grade command.
type CreateCommand struct {
	Code           string
	Name           string
	Description    string
	BCPerc         float64
	NonStdPerc     float64
	BCRecoveryRate float64
	Notes          string
	CreatedBy      string
}

// CreateHandler handles the CreateProductGrade command.
type CreateHandler struct {
	repo productgrade.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo productgrade.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create Product Grade command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*productgrade.Entity, error) {
	exists, err := h.repo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, productgrade.ErrAlreadyExists
	}

	entity, err := productgrade.New(
		cmd.Code, cmd.Name, cmd.Description,
		cmd.BCPerc, cmd.NonStdPerc, cmd.BCRecoveryRate,
		cmd.Notes, cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
