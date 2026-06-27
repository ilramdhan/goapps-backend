// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// CreateCommand is the input for creating a new Box Bobbin Cost.
type CreateCommand struct {
	Code         string
	Name         string
	BBCType      string
	NoOfBob      int
	Notes        string
	BbnReuse     *float64
	BoxReuse     *float64
	BoxCost      *float64
	BobinCost    *float64
	BoxCostVal   *float64
	BobinCostVal *float64
	BbnReuseVal  *float64
	BoxReuseVal  *float64
	CreatedBy    string
}

// CreateHandler handles the CreateBoxBobbinCost command.
type CreateHandler struct {
	repo boxbobbincost.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo boxbobbincost.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*boxbobbincost.Entity, error) {
	entity, err := boxbobbincost.New(cmd.Code, cmd.Name, cmd.BBCType, cmd.NoOfBob, cmd.Notes,
		cmd.BbnReuse, cmd.BoxReuse, cmd.BoxCost, cmd.BobinCost, cmd.BoxCostVal, cmd.BobinCostVal,
		cmd.BbnReuseVal, cmd.BoxReuseVal,
		cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, fmt.Errorf("create box bobbin cost: %w", err)
	}

	return entity, nil
}
