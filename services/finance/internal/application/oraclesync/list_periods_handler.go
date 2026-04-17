package oraclesync

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// ListPeriodsHandler retrieves distinct synced periods.
type ListPeriodsHandler struct {
	pgRepo syncdata.PostgresTargetRepository
}

// NewListPeriodsHandler creates a new ListPeriodsHandler.
func NewListPeriodsHandler(pgRepo syncdata.PostgresTargetRepository) *ListPeriodsHandler {
	return &ListPeriodsHandler{pgRepo: pgRepo}
}

// Handle retrieves all distinct periods.
func (h *ListPeriodsHandler) Handle(ctx context.Context) ([]string, error) {
	periods, err := h.pgRepo.GetDistinctPeriods(ctx)
	if err != nil {
		return nil, fmt.Errorf("get distinct periods: %w", err)
	}
	return periods, nil
}
