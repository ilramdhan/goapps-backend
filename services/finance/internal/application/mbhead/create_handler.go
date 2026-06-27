// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// CreateCommand represents the create MB Head command.
type CreateCommand struct {
	MBCosting       string
	OracleSysID     *string
	MgtName         *string
	Denier          *float64
	Filament        *int
	Dozing          *float64
	MBHCheckStatus  *string
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHFinalProduct *string
	MBHCode         *string
	CreatedBy       string
}

// CreateHandler handles the CreateMBHead command.
type CreateHandler struct {
	repo mbhead.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mbhead.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB Head command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbhead.Entity, error) {
	exists, err := h.repo.ExistsByMBCosting(ctx, cmd.MBCosting)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, mbhead.ErrAlreadyExists
	}

	entity, err := mbhead.New(
		cmd.MBCosting, cmd.OracleSysID, cmd.MgtName,
		cmd.Denier, cmd.Filament, cmd.Dozing,
		cmd.MBHCheckStatus, cmd.MBHStatus, cmd.MBHLdrPrsn, cmd.MBHFinalProduct, cmd.MBHCode,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
