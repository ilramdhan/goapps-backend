// Package yarntxweightgroup provides application layer handlers for TX Weight groups.
package yarntxweightgroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// GetQuery represents the get TX Weight group query.
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles GetYarnTxWeightGroup.
type GetHandler struct {
	repo yarntxweightgroup.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo yarntxweightgroup.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle returns the group by id.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*yarntxweightgroup.Entity, error) {
	return h.repo.GetByID(ctx, q.ID)
}
