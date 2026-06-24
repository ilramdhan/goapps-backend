// Package machine provides application layer handlers for Machine operations.
package machine

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// CreateCommand holds the input for creating a new machine.
type CreateCommand struct {
	Code          string
	Name          string
	MCType        string
	Location      string
	NoOfPosition  int
	NoOfEnd       int
	MCSpeed       float64
	MachineRPM    *float64
	MCEfficiency  float64
	PowerPerDay   *float64
	MpPerDay      *float64
	OhsPerDay     *float64
	SparesPerDay  *float64
	KgsLostChange *float64
	Vb1Qty        *float64
	Vb2Qty        *float64
	Vb3Qty        *float64
	Vb4Qty        *float64
	Vb5Qty        *float64
	Notes         string
	CreatedBy     string
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
		cmd.MCEfficiency, cmd.PowerPerDay,
		cmd.MpPerDay, cmd.OhsPerDay, cmd.SparesPerDay, cmd.KgsLostChange,
		cmd.Vb1Qty, cmd.Vb2Qty, cmd.Vb3Qty, cmd.Vb4Qty, cmd.Vb5Qty,
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
