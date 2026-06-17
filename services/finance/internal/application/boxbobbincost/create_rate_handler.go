// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// CreateRateCommand represents the command to add a rate entry to a BBC config.
type CreateRateCommand struct {
	ParentID   uuid.UUID
	Period     string
	BobRateMkt float64
	BoxRateMkt float64
	BobRateVal *float64
	BoxRateVal *float64
	CreatedBy  string
}

// CreateRateHandler handles the CreateBoxBobbinCostRate command.
type CreateRateHandler struct {
	repo boxbobbincost.Repository
}

// NewCreateRateHandler creates a new CreateRateHandler.
func NewCreateRateHandler(repo boxbobbincost.Repository) *CreateRateHandler {
	return &CreateRateHandler{repo: repo}
}

// Handle executes the create rate command.
func (h *CreateRateHandler) Handle(ctx context.Context, cmd CreateRateCommand) (*boxbobbincost.RateEntry, error) {
	rate := boxbobbincost.NewRateEntry(cmd.ParentID, cmd.Period, cmd.BobRateMkt, cmd.BoxRateMkt, cmd.BobRateVal, cmd.BoxRateVal, cmd.CreatedBy)
	if err := h.repo.CreateRate(ctx, rate); err != nil {
		return nil, err
	}
	return rate, nil
}
