// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery is the paginated list query for heads.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	Flag      string
	SortBy    string
	SortOrder string
}

// ListResult is the paginated list result.
type ListResult struct {
	Heads       []*rmgroup.Head
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles ListHeads queries.
type ListHandler struct {
	repo rmgroup.Repository
}

// NewListHandler builds a ListHandler.
func NewListHandler(repo rmgroup.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list query with pagination and filtering. An empty Flag is
// treated as "no flag filter"; otherwise the value must parse to a valid Flag.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := rmgroup.ListFilter{
		Search:    query.Search,
		IsActive:  query.IsActive,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	if query.Flag != "" {
		flag, err := rmgroup.ParseFlag(query.Flag)
		if err != nil {
			return nil, err
		}
		filter.Flag = flag
	}

	filter.Validate()

	heads, total, err := h.repo.ListHeads(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list heads: %w", err)
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Heads:       heads,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
