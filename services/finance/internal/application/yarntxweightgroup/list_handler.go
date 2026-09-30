// Package yarntxweightgroup provides application layer handlers for TX Weight groups.
package yarntxweightgroup

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list TX Weight groups query. Its fields mirror
// yarntxweightgroup.ListFilter exactly so it converts directly.
type ListQuery struct {
	Search        string
	ProductTypeID int32
	Page          int
	PageSize      int
	SortBy        string
	SortOrder     string
}

// ListResult represents the list result.
type ListResult struct {
	Items       []*yarntxweightgroup.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles ListYarnTxWeightGroups.
type ListHandler struct {
	repo yarntxweightgroup.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo yarntxweightgroup.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle lists groups with filtering and pagination.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := yarntxweightgroup.ListFilter(q)
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
