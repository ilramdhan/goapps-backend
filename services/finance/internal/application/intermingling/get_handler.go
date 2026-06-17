// Package intermingling provides application layer handlers for Intermingling operations.
package intermingling

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// GetQuery represents the get Intermingling query.
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles the GetIntermingling query.
type GetHandler struct {
	repo intermingling.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo intermingling.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get Intermingling query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*intermingling.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
