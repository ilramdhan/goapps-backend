// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// GetQuery represents the get MB Head query.
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles the GetMBHead query.
type GetHandler struct {
	repo mbhead.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo mbhead.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get MB Head query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*mbhead.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
