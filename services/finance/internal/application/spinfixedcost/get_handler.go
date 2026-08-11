// Package spinfixedcost provides application layer handlers for Spin Fixed Cost operations.
package spinfixedcost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

// GetQuery represents the get Spin Fixed Cost query.
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles the GetSpinFixedCost query.
type GetHandler struct {
	repo spinfixedcost.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo spinfixedcost.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get Spin Fixed Cost query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*spinfixedcost.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
