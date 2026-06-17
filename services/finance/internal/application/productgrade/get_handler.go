// Package productgrade provides application layer handlers for Product Grade operations.
package productgrade

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// GetQuery represents the get Product Grade query.
type GetQuery struct {
	ProductGradeID uuid.UUID
}

// GetHandler handles the GetProductGrade query.
type GetHandler struct {
	repo productgrade.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo productgrade.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get Product Grade query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*productgrade.Entity, error) {
	return h.repo.GetByID(ctx, query.ProductGradeID)
}
