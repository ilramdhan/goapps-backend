// Package shade provides application layer handlers for the shade master (R8).
package shade

import (
	"context"
	"errors"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// CreateCommand represents the create shade command (manual/hand-authored shade).
type CreateCommand struct {
	Code      string
	Name      string
	ShortName *string
	CreatedBy string
}

// CreateHandler handles the CreateShade command.
type CreateHandler struct {
	repo shade.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo shade.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create shade command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*shade.Shade, error) {
	existing, err := h.repo.GetByCode(ctx, cmd.Code)
	if err != nil && !errors.Is(err, shade.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, shade.ErrAlreadyExists
	}

	entity, err := shade.New(shade.NewParams{
		Code: cmd.Code, Name: cmd.Name, ShortName: cmd.ShortName, CreatedBy: cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
