// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list Parameters query.
type ListQuery struct {
	Page          int
	PageSize      int
	Search        string
	DataType      *string
	ParamCategory *string
	IsActive      *bool
	SortBy        string
	SortOrder     string
}

// ListResult represents the list Parameters result.
type ListResult struct {
	Parameters  []*parameter.Parameter
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListParameters query.
type ListHandler struct {
	repo parameter.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo parameter.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list Parameters query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	// Build filter
	filter := parameter.ListFilter{
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// DataType filter
	if query.DataType != nil {
		dt, err := parameter.NewDataType(*query.DataType)
		if err != nil {
			return nil, err
		}
		filter.DataType = &dt
	}

	// ParamCategory filter
	if query.ParamCategory != nil {
		cat, err := parameter.NewParamCategory(*query.ParamCategory)
		if err != nil {
			return nil, err
		}
		filter.ParamCategory = &cat
	}

	// IsActive filter
	filter.IsActive = query.IsActive

	// Validate filter
	filter.Validate()

	// Execute query
	params, total, err := h.repo.List(ctx, filter)
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
		Parameters:  params,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
