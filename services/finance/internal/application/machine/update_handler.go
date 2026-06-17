// Package machine provides application layer handlers for Machine operations.
package machine

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// UpdateCommand holds the input for updating an existing machine.
type UpdateCommand struct {
	MachineID    uuid.UUID
	Name         *string
	MCType       *string
	Location     *string
	NoOfPosition *int
	NoOfEnd      *int
	MCSpeed      *float64
	MachineRPM   *float64
	MCEfficiency *float64
	PowerPerDay  *float64
	Notes        *string
	IsActive     *bool
	UpdatedBy    string
}

// UpdateHandler handles the UpdateMachine command.
type UpdateHandler struct {
	repo machine.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo machine.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update machine command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*machine.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MachineID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(machine.UpdateInput{
		Name:         cmd.Name,
		MCType:       cmd.MCType,
		Location:     cmd.Location,
		NoOfPosition: cmd.NoOfPosition,
		NoOfEnd:      cmd.NoOfEnd,
		MCSpeed:      cmd.MCSpeed,
		MachineRPM:   cmd.MachineRPM,
		MCEfficiency: cmd.MCEfficiency,
		PowerPerDay:  cmd.PowerPerDay,
		Notes:        cmd.Notes,
		IsActive:     cmd.IsActive,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
