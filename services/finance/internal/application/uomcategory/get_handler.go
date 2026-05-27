// Package uomcategory provides application layer handlers for UOM Category operations.
package uomcategory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

// GetQuery represents the get UOM Category query.
type GetQuery struct {
	UOMCategoryID string
}

// GetHandler handles the GetUOMCategory query.
type GetHandler struct {
	repo uomcategory.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo uomcategory.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get UOM Category query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*uomcategory.Category, error) {
	id, err := uuid.Parse(query.UOMCategoryID)
	if err != nil {
		return nil, uomcategory.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
