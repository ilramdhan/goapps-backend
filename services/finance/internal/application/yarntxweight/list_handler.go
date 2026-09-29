// Package yarntxweight provides application layer handlers for the TX Weight master.
package yarntxweight

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list TX Weight rules query.
type ListQuery struct {
	Page          int
	PageSize      int
	Search        string
	ProductTypeID int32
	Grade         yarntxweight.Grade
	SortBy        string
	SortOrder     string
}

// ListResult represents the list result.
type ListResult struct {
	Items       []*yarntxweight.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles ListYarnTxWeights.
type ListHandler struct {
	repo yarntxweight.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo yarntxweight.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle lists rules with filtering and pagination.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := yarntxweight.ListFilter{
		Search:        q.Search,
		ProductTypeID: q.ProductTypeID,
		Grade:         q.Grade,
		Page:          q.Page,
		PageSize:      q.PageSize,
		SortBy:        q.SortBy,
		SortOrder:     q.SortOrder,
	}
	filter.Validate()

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}
	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
