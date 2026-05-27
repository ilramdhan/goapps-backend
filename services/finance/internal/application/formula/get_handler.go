// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// GetQuery represents the get Formula query.
type GetQuery struct {
	FormulaID string
}

// GetHandler handles the GetFormula query.
type GetHandler struct {
	repo formula.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo formula.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get Formula query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*formula.Formula, error) {
	id, err := uuid.Parse(query.FormulaID)
	if err != nil {
		return nil, formula.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
