package superbacostsp

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// GetHandler handles GetSuperbaCostSp.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler creates a GetHandler.
func NewGetHandler(repo domain.Repository) *GetHandler { return &GetHandler{repo: repo} }

// Handle fetches a row by id.
func (h *GetHandler) Handle(ctx context.Context, id string) (*domain.Entry, error) {
	return h.repo.GetByID(ctx, id)
}
