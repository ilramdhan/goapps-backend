// Package spinfixedcost provides application layer handlers for Spin Fixed Cost operations.
package spinfixedcost

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

// CreateCommand represents the create Spin Fixed Cost command.
type CreateCommand struct {
	Period             string
	CommonPoyDenier    float64
	PoyProduction      float64
	SpinPowerMonth     float64
	SpinManpowerMonth  float64
	SpinOverheadsMonth float64
	SpinConssprsMonth  float64
	CreatedBy          string
}

// CreateHandler handles the CreateSpinFixedCost command.
type CreateHandler struct {
	repo spinfixedcost.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo spinfixedcost.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create Spin Fixed Cost command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*spinfixedcost.Entity, error) {
	// Pre-check for a friendly error; the repository still catches 23505 from the
	// partial unique index in case of a concurrent insert.
	exists, err := h.repo.ExistsByPeriod(ctx, cmd.Period)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, spinfixedcost.ErrDuplicatePeriod
	}

	entity, err := spinfixedcost.New(spinfixedcost.NewInput{
		Period:             cmd.Period,
		CommonPoyDenier:    cmd.CommonPoyDenier,
		PoyProduction:      cmd.PoyProduction,
		SpinPowerMonth:     cmd.SpinPowerMonth,
		SpinManpowerMonth:  cmd.SpinManpowerMonth,
		SpinOverheadsMonth: cmd.SpinOverheadsMonth,
		SpinConssprsMonth:  cmd.SpinConssprsMonth,
		CreatedBy:          cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
