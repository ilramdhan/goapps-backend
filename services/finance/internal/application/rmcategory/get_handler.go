// Package rmcategory provides application layer handlers for RMCategory operations.
package rmcategory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// GetQuery represents the get RMCategory query.
type GetQuery struct {
	RMCategoryID string
}

// GetHandler handles the GetRMCategory query.
type GetHandler struct {
	repo rmcategory.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo rmcategory.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get RMCategory query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*rmcategory.RMCategory, error) {
	id, err := uuid.Parse(query.RMCategoryID)
	if err != nil {
		return nil, rmcategory.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
