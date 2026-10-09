package superbacostsp

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// DeleteHandler handles DeleteSuperbaCostSp (soft delete).
type DeleteHandler struct{ repo domain.Repository }

// NewDeleteHandler creates a DeleteHandler.
func NewDeleteHandler(repo domain.Repository) *DeleteHandler { return &DeleteHandler{repo: repo} }

// Handle soft-deletes a row.
func (h *DeleteHandler) Handle(ctx context.Context, id, deletedBy string) error {
	return h.repo.SoftDelete(ctx, id, deletedBy)
}
