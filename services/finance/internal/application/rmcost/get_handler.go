// Package rmcost provides application layer handlers for RM landed-cost calculation jobs.
package rmcost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// GetQuery retrieves a single cost row by ID, or by (period, rm_code).
type GetQuery struct {
	CostID string
	Period string
	RMCode string
}

// GetHandler handles GetCost queries.
type GetHandler struct {
	repo rmcost.Repository
}

// NewGetHandler builds a GetHandler.
func NewGetHandler(repo rmcost.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle prefers CostID when non-empty, otherwise falls back to (period, rm_code).
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*rmcost.Cost, error) {
	if query.CostID != "" {
		id, err := uuid.Parse(query.CostID)
		if err != nil {
			return nil, rmcost.ErrNotFound
		}
		return h.repo.GetByID(ctx, id)
	}
	return h.repo.GetByPeriodAndCode(ctx, query.Period, query.RMCode)
}
