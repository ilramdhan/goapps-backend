// Package rmcost provides application layer handlers for RM landed-cost calculation jobs.
package rmcost

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery is the paginated list query for cost rows.
type ListQuery struct {
	Page        int
	PageSize    int
	Period      string
	RMType      string
	GroupHeadID string
	Search      string
	SortBy      string
	SortOrder   string
}

// ListResult is the paginated list result.
type ListResult struct {
	Costs       []*rmcost.Cost
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles ListCosts queries.
type ListHandler struct {
	repo rmcost.Repository
}

// NewListHandler builds a ListHandler.
func NewListHandler(repo rmcost.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle applies filters, runs the repository query, and wraps the result with
// pagination metadata suitable for the standard BaseResponse envelope.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := rmcost.ListFilter{
		Period:    query.Period,
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	if query.RMType != "" {
		rmType := rmcost.RMType(query.RMType)
		if !rmType.IsValid() {
			return nil, rmcost.ErrInvalidRMType
		}
		filter.RMType = rmType
	}

	if query.GroupHeadID != "" {
		id, err := uuid.Parse(query.GroupHeadID)
		if err != nil {
			return nil, fmt.Errorf("invalid group head id: %w", err)
		}
		filter.GroupHeadID = &id
	}

	filter.Validate()

	costs, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list costs: %w", err)
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Costs:       costs,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
