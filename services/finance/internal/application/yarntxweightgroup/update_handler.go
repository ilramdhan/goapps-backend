// Package yarntxweightgroup provides application layer handlers for TX Weight groups.
package yarntxweightgroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// UpdateCommand represents the update TX Weight group command. Input is the
// full replacement state: the type set and the rules are replaced, not merged.
type UpdateCommand struct {
	ID        uuid.UUID
	Input     yarntxweightgroup.Input
	UpdatedBy string
}

// UpdateHandler handles UpdateYarnTxWeightGroup.
type UpdateHandler struct {
	repo yarntxweightgroup.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo yarntxweightgroup.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle replaces the group state and persists it in one transaction.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*yarntxweightgroup.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(cmd.Input, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := checkInvariants(ctx, h.repo, entity, entity.ID()); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return h.repo.GetByID(ctx, entity.ID())
}
