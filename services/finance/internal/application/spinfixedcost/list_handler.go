// Package spinfixedcost provides application layer handlers for Spin Fixed Cost operations.
package spinfixedcost

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list Spin Fixed Cost query.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	Period    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult represents the list Spin Fixed Cost result.
type ListResult struct {
	Items       []*spinfixedcost.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListSpinFixedCosts query.
type ListHandler struct {
	repo spinfixedcost.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo spinfixedcost.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list Spin Fixed Cost query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := spinfixedcost.ListFilter{
		Search:    query.Search,
		Period:    query.Period,
		IsActive:  query.IsActive,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}
	filter.Validate()

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
