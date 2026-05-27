// Package uomcategory provides application layer handlers for UOM Category operations.
package uomcategory

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list UOM Categories query.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult represents the list UOM Categories result.
type ListResult struct {
	Categories  []*uomcategory.Category
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListUOMCategories query.
type ListHandler struct {
	repo uomcategory.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo uomcategory.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list UOM Categories query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	// Build filter
	filter := uomcategory.ListFilter{
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
		IsActive:  query.IsActive,
	}

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
