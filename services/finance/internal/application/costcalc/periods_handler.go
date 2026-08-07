package costcalc

import (
	"context"
	"fmt"
)

// PeriodsHandler returns the set of distinct periods with cost results.
type PeriodsHandler struct {
	svc *Service
}

// NewPeriodsHandler constructs the handler.
func NewPeriodsHandler(svc *Service) *PeriodsHandler {
	return &PeriodsHandler{svc: svc}
}

// Handle returns distinct periods newest-first.
func (h *PeriodsHandler) Handle(ctx context.Context) ([]string, error) {
	periods, err := h.svc.resultRepo.ListDistinctPeriods(ctx)
	if err != nil {
		return nil, fmt.Errorf("list distinct cost result periods: %w", err)
	}
	return periods, nil
}
