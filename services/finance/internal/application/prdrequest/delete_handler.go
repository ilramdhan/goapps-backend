// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// DeleteCommand carries inputs to DeleteHandler.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler soft-deletes a product request by its UUID.
type DeleteHandler struct {
	repo domainprdrequest.Repository
}

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(repo domainprdrequest.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle soft-deletes the request identified by cmd.ID.
// Returns ErrNotFound if the request does not exist.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.DeletedBy)
}
