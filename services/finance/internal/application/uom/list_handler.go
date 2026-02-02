// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// ListQuery represents the list UOMs query.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	Category  *string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult represents the list UOMs result.
type ListResult struct {
	UOMs        []*uom.UOM
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListUOMs query.
type ListHandler struct {
	repo uom.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo uom.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list UOMs query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	// Build filter
	filter := uom.ListFilter{
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// Category filter
	if query.Category != nil {
		cat, err := uom.NewCategory(*query.Category)
		if err != nil {
			return nil, err
		}
		filter.Category = &cat
	}

	// IsActive filter
	filter.IsActive = query.IsActive

	// Validate filter
	filter.Validate()

	// Execute query
	uoms, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Calculate pages
	totalPages := int32(0)
	if filter.PageSize > 0 {
		totalPages = int32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}

	return &ListResult{
		UOMs:        uoms,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: int32(filter.Page),
		PageSize:    int32(filter.PageSize),
	}, nil
}
