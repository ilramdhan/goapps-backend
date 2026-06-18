// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// GetQuery is the input for retrieving a single Box Bobbin Cost (includes rates).
type GetQuery struct {
	ID uuid.UUID
}

// GetHandler handles the GetBoxBobbinCost query.
type GetHandler struct {
	repo boxbobbincost.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo boxbobbincost.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get query and returns the entity with its rates.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*boxbobbincost.Entity, error) {
	return h.repo.GetByID(ctx, query.ID)
}
