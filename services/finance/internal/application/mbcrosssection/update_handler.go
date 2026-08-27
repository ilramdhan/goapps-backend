package mbcrosssection

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// UpdateCommand represents the update MB cross-section command.
type UpdateCommand struct {
	ID           string
	DisplayName  string
	Description  string
	DisplayOrder int32
	IsActive     bool
	UpdatedBy    string
}

// UpdateHandler handles the UpdateMbCrossSection command.
type UpdateHandler struct {
	repo mbcrosssection.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbcrosssection.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB cross-section command. The code is immutable — it is
// referenced by mst_mb_cross_section_factor via a foreign key.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbcrosssection.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.DisplayName, cmd.Description, cmd.DisplayOrder, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
