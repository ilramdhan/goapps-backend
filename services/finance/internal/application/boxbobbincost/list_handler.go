// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery is the input for listing Box Bobbin Costs.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult holds the paginated list result.
type ListResult struct {
	Items       []*boxbobbincost.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListBoxBobbinCosts query.
type ListHandler struct {
	repo boxbobbincost.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo boxbobbincost.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := boxbobbincost.ListFilter{
		Search:    query.Search,
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
