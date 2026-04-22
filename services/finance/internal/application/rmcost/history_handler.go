// Package rmcost provides application layer handlers for RM landed-cost calculation jobs.
package rmcost

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// HistoryQuery pages through the append-only aud_rm_cost_history trail.
type HistoryQuery struct {
	Page        int
	PageSize    int
	Period      string
	RMCode      string
	GroupHeadID string
	JobID       string
}

// HistoryResult is the paginated history result.
type HistoryResult struct {
	Rows        []rmcost.History
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// HistoryHandler returns rows from aud_rm_cost_history.
type HistoryHandler struct {
	repo rmcost.Repository
}

// NewHistoryHandler builds a HistoryHandler.
func NewHistoryHandler(repo rmcost.Repository) *HistoryHandler {
	return &HistoryHandler{repo: repo}
}

// Handle parses optional UUIDs, applies defaults, and returns the matching rows
// newest-first.
func (h *HistoryHandler) Handle(ctx context.Context, query HistoryQuery) (*HistoryResult, error) {
	filter := rmcost.HistoryFilter{
		Period:   query.Period,
		RMCode:   query.RMCode,
		Page:     query.Page,
		PageSize: query.PageSize,
	}
	if query.GroupHeadID != "" {
		id, err := uuid.Parse(query.GroupHeadID)
		if err != nil {
			return nil, fmt.Errorf("invalid group head id: %w", err)
		}
		filter.GroupHeadID = &id
	}
	if query.JobID != "" {
		id, err := uuid.Parse(query.JobID)
		if err != nil {
			return nil, fmt.Errorf("invalid job id: %w", err)
		}
		filter.JobID = &id
	}
	filter.Validate()

	rows, total, err := h.repo.ListHistory(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &HistoryResult{
		Rows:        rows,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
