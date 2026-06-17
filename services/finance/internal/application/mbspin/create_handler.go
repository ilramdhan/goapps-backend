// Package mbspin provides application layer handlers for MB Spin operations.
package mbspin

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// CreateCommand represents the create MB Spin command.
type CreateCommand struct {
	HeadID      uuid.UUID
	MgtName     string
	OracleSysID *string
	Denier      *float64
	Filament    *int
	Dozing      *float64
	MBCosting   *string
	CreatedBy   string
}

// CreateHandler handles the CreateMBSpin command.
type CreateHandler struct {
	repo mbspin.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mbspin.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB Spin command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbspin.Entity, error) {
	entity, err := mbspin.New(
		cmd.HeadID, cmd.MgtName, cmd.OracleSysID,
		cmd.Denier, cmd.Filament, cmd.Dozing, cmd.MBCosting,
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
