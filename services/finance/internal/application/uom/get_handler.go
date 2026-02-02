// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// GetQuery represents the get UOM query.
type GetQuery struct {
	UOMID string
}

// GetHandler handles the GetUOM query.
type GetHandler struct {
	repo uom.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo uom.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get UOM query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*uom.UOM, error) {
	id, err := uuid.Parse(query.UOMID)
	if err != nil {
		return nil, uom.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
