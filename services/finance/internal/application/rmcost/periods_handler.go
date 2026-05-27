package rmcost

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// PeriodsHandler returns the set of distinct periods with cost rows.
type PeriodsHandler struct {
	repo rmcost.Repository
}

// NewPeriodsHandler builds a PeriodsHandler.
func NewPeriodsHandler(repo rmcost.Repository) *PeriodsHandler {
	return &PeriodsHandler{repo: repo}
}

// Handle returns distinct periods newest-first.
func (h *PeriodsHandler) Handle(ctx context.Context) ([]string, error) {
	periods, err := h.repo.ListDistinctPeriods(ctx)
	if err != nil {
		return nil, fmt.Errorf("list distinct periods: %w", err)
	}
	return periods, nil
}
