// Package yarntxweight provides application layer handlers for the TX Weight master.
package yarntxweight

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// GetQuery represents the get TX Weight rule query.
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles GetYarnTxWeight.
type GetHandler struct {
	repo yarntxweight.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo yarntxweight.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle returns the rule by id.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*yarntxweight.Entity, error) {
	return h.repo.GetByID(ctx, q.ID)
}
