// Package shade provides application layer handlers for the shade master (R8).
package shade

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list shades query.
type ListQuery struct {
	Page         int
	PageSize     int
	Search       string
	IsActive     *bool
	SourceFilter string
	SortBy       string
	SortOrder    string
}

// ListResult represents the list shades result.
type ListResult struct {
	Shades      []*shade.Shade
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListShades query.
type ListHandler struct {
	repo shade.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo shade.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list shades query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := shade.ListFilter{
		Search:       query.Search,
		IsActive:     query.IsActive,
		SourceFilter: query.SourceFilter,
		Page:         query.Page,
		PageSize:     query.PageSize,
		SortBy:       query.SortBy,
		SortOrder:    query.SortOrder,
	}
	filter.Validate()

	shades, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Shades:      shades,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
