// Package shade provides application layer handlers for the shade master (R8).
package shade

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// GetHandler handles fetching a single shade.
type GetHandler struct {
	repo shade.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo shade.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// HandleByID fetches a shade by its ID.
func (h *GetHandler) HandleByID(ctx context.Context, id int64) (*shade.Shade, error) {
	return h.repo.GetByID(ctx, id)
}

// HandleByCode fetches a shade by its code.
func (h *GetHandler) HandleByCode(ctx context.Context, code string) (*shade.Shade, error) {
	return h.repo.GetByCode(ctx, code)
}
