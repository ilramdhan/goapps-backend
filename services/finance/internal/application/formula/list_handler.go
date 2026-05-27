// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list Formulas query.
type ListQuery struct {
	Page        int
	PageSize    int
	Search      string
	FormulaType *string
	IsActive    *bool
	SortBy      string
	SortOrder   string
}

// ListResult represents the list Formulas result.
type ListResult struct {
	Formulas    []*formula.Formula
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListFormulas query.
type ListHandler struct {
	repo formula.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo formula.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list Formulas query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := formula.ListFilter{
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	if query.FormulaType != nil {
		ft, err := formula.NewType(*query.FormulaType)
		if err != nil {
			return nil, err
		}
		filter.FormulaType = &ft
	}

	filter.IsActive = query.IsActive
	filter.Validate()

	formulas, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Formulas:    formulas,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
