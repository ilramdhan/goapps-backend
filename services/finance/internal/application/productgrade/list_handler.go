// Package productgrade provides application layer handlers for Product Grade operations.
package productgrade

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list Product Grades query.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult represents the list Product Grades result.
type ListResult struct {
	Grades      []*productgrade.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListProductGrades query.
type ListHandler struct {
	repo productgrade.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo productgrade.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list Product Grades query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := productgrade.ListFilter{
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
		IsActive:  query.IsActive,
	}

	filter.Validate()

	grades, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Grades:      grades,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
