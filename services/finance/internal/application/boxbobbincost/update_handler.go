// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// UpdateCommand is the input for updating a Box Bobbin Cost.
type UpdateCommand struct {
	ID           uuid.UUID
	Name         *string
	BBCType      *string
	NoOfBob      *int
	Notes        *string
	IsActive     *bool
	BbnReuse     *float64
	BoxReuse     *float64
	BoxCost      *float64
	BobinCost    *float64
	BoxCostVal   *float64
	BobinCostVal *float64
	BbnReuseVal  *float64
	BoxReuseVal  *float64
	UpdatedBy    string
}

// UpdateHandler handles the UpdateBoxBobbinCost command.
type UpdateHandler struct {
	repo boxbobbincost.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo boxbobbincost.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*boxbobbincost.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(boxbobbincost.UpdateInput{
		Name:         cmd.Name,
		BBCType:      cmd.BBCType,
		NoOfBob:      cmd.NoOfBob,
		Notes:        cmd.Notes,
		IsActive:     cmd.IsActive,
		BbnReuse:     cmd.BbnReuse,
		BoxReuse:     cmd.BoxReuse,
		BoxCost:      cmd.BoxCost,
		BobinCost:    cmd.BobinCost,
		BoxCostVal:   cmd.BoxCostVal,
		BobinCostVal: cmd.BobinCostVal,
		BbnReuseVal:  cmd.BbnReuseVal,
		BoxReuseVal:  cmd.BoxReuseVal,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, fmt.Errorf("update box bobbin cost: %w", err)
	}

	return entity, nil
}
