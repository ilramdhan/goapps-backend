// Package productgrade provides application layer handlers for Product Grade operations.
package productgrade

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// UpdateCommand represents the update Product Grade command.
type UpdateCommand struct {
	ProductGradeID uuid.UUID
	Name           *string
	Description    *string
	BCPerc         *float64
	NonStdPerc     *float64
	BCRecoveryRate *float64
	Notes          *string
	IsActive       *bool
	UpdatedBy      string
}

// UpdateHandler handles the UpdateProductGrade command.
type UpdateHandler struct {
	repo productgrade.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo productgrade.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Product Grade command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*productgrade.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ProductGradeID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(productgrade.UpdateInput{
		Name:           cmd.Name,
		Description:    cmd.Description,
		BCPerc:         cmd.BCPerc,
		NonStdPerc:     cmd.NonStdPerc,
		BCRecoveryRate: cmd.BCRecoveryRate,
		Notes:          cmd.Notes,
		IsActive:       cmd.IsActive,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
