package mbcrosssection

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// GetQuery represents the get MB cross-section query.
type GetQuery struct {
	ID string
}

// GetHandler handles the GetMbCrossSection query.
type GetHandler struct {
	repo mbcrosssection.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo mbcrosssection.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get MB cross-section query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*mbcrosssection.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
