// Package machine provides application layer handlers for Machine operations.
package machine

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// CreateCommand holds the input for creating a new machine.
type CreateCommand struct {
	Code         string
	Name         string
	MCType       string
	Location     string
	NoOfPosition int
	NoOfEnd      int
	MCSpeed      float64
	MachineRPM   *float64
	MCEfficiency float64
	PowerPerDay  *float64
	Notes        string
	CreatedBy    string
}

// CreateHandler handles the CreateMachine command.
type CreateHandler struct {
	repo machine.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo machine.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create machine command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*machine.Entity, error) {
	exists, err := h.repo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, machine.ErrAlreadyExists
	}

	entity, err := machine.New(
		cmd.Code, cmd.Name, cmd.MCType, cmd.Location,
		cmd.NoOfPosition, cmd.NoOfEnd, cmd.MCSpeed, cmd.MachineRPM,
		cmd.MCEfficiency, cmd.PowerPerDay, cmd.Notes, cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
