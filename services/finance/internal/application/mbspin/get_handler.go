// Package mbspin provides application layer handlers for MB Spin operations.
package mbspin

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// GetQuery represents the get MB Spin query.
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles the GetMBSpin query.
type GetHandler struct {
	repo mbspin.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo mbspin.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get MB Spin query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*mbspin.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
