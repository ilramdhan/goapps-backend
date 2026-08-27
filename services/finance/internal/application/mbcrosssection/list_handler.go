package mbcrosssection

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list MB cross-section query.
type ListQuery struct {
	Page      int32
	PageSize  int32
	Search    string
	SortBy    string
	SortOrder string
	IsActive  *bool
}

// ListResult represents the list MB cross-section result.
type ListResult struct {
	Items       []*mbcrosssection.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListMbCrossSection query.
type ListHandler struct {
	repo mbcrosssection.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo mbcrosssection.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list MB cross-section query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := mbcrosssection.ListFilter{
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

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages(total, filter.PageSize),
		CurrentPage: filter.Page,
		PageSize:    filter.PageSize,
	}, nil
}

// totalPages computes the page count for a total row count and page size.
func totalPages(total int64, pageSize int32) int32 {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	computed := (total + int64(pageSize) - 1) / int64(pageSize)
	return safeconv.Int64ToInt32(computed)
}
