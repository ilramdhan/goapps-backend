// Package yarntxweight provides application layer handlers for the TX Weight master.
package yarntxweight

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// CreateCommand represents the create TX Weight rule command.
type CreateCommand struct {
	ProductTypeID int32
	Grade         yarntxweight.Grade
	Mode          yarntxweight.Mode
	Value         float64
	Description   string
	CreatedBy     string
}

// CreateHandler handles CreateYarnTxWeight.
type CreateHandler struct {
	repo yarntxweight.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo yarntxweight.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle validates, checks uniqueness and persists a new rule.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*yarntxweight.Entity, error) {
	entity, err := yarntxweight.New(cmd.ProductTypeID, cmd.Grade, cmd.Mode, cmd.Value, cmd.Description, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	typeExists, err := h.repo.ProductTypeExists(ctx, cmd.ProductTypeID)
	if err != nil {
		return nil, err
	}
	if !typeExists {
		return nil, yarntxweight.ErrProductTypeNotFound
	}

	exists, err := h.repo.ExistsByTypeGrade(ctx, cmd.ProductTypeID, cmd.Grade)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, yarntxweight.ErrAlreadyExists
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	// Re-read so the response carries the joined product type code/name.
	return h.repo.GetByID(ctx, entity.ID())
}
