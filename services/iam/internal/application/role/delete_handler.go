// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// DeleteCommand represents the delete role command.
type DeleteCommand struct {
	RoleID    string
	DeletedBy string
}

// DeleteHandler handles the DeleteRole command.
type DeleteHandler struct {
	repo role.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo role.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete role command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.RoleID)
	if err != nil {
		return shared.ErrNotFound
	}

	// 2. Get existing entity to check if it is a system role
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 3. Prevent system role deletion via domain logic
	if err := entity.SoftDelete(cmd.DeletedBy); err != nil {
		return err
	}

	// 4. Persist deletion
	return h.repo.Delete(ctx, id, cmd.DeletedBy)
}
