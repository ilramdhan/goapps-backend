// Package yarntxweightgroup provides application layer handlers for TX Weight
// groups (one config shared by many product types).
package yarntxweightgroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// CreateCommand represents the create TX Weight group command.
type CreateCommand struct {
	Input     yarntxweightgroup.Input
	CreatedBy string
}

// CreateHandler handles CreateYarnTxWeightGroup.
type CreateHandler struct {
	repo yarntxweightgroup.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo yarntxweightgroup.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle validates, checks code / product type ownership and persists a group.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*yarntxweightgroup.Entity, error) {
	entity, err := yarntxweightgroup.New(cmd.Input, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := checkInvariants(ctx, h.repo, entity, uuid.Nil); err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	// Re-read so the response carries the joined product type code/name.
	return h.repo.GetByID(ctx, entity.ID())
}

// checkInvariants enforces the cross-aggregate rules the entity cannot see:
// unique live code, existing product types, and one group per product type.
// The DB unique indexes back all three against races.
func checkInvariants(ctx context.Context, repo yarntxweightgroup.Repository, e *yarntxweightgroup.Entity, excludeID uuid.UUID) error {
	exists, err := repo.ExistsByCode(ctx, e.Code(), excludeID)
	if err != nil {
		return err
	}
	if exists {
		return yarntxweightgroup.ErrCodeAlreadyExists
	}
	ids := e.ProductTypeIDs()
	missing, err := repo.MissingProductTypes(ctx, ids)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return yarntxweightgroup.ErrProductTypeNotFound
	}
	conflicts, err := repo.FindProductTypeConflicts(ctx, ids, excludeID)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return &yarntxweightgroup.ProductTypeConflictError{Conflicts: conflicts}
	}
	return nil
}
