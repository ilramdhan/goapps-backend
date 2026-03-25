// Package rmcategory provides application layer handlers for RMCategory operations.
package rmcategory

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list RMCategories query.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult represents the list RMCategories result.
type ListResult struct {
	Categories  []*rmcategory.RMCategory
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListRMCategories query.
type ListHandler struct {
	repo rmcategory.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo rmcategory.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list RMCategories query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	// Build filter
	filter := rmcategory.ListFilter{
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// IsActive filter
	filter.IsActive = query.IsActive

	// Validate filter
	filter.Validate()

	// Execute query
	categories, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Calculate total pages using safe conversion
	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Categories:  categories,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
