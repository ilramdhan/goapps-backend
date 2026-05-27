// Package employeegroup provides application layer handlers for employee group operations.
package employeegroup

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// ListQuery is the query for listing employee groups.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult is the result of listing employee groups.
type ListResult struct {
	Items       []*employeegroup.EmployeeGroup
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles ListEmployeeGroups queries.
type ListHandler struct {
	repo employeegroup.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo employeegroup.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	params := employeegroup.ListParams{
		Page:      page,
		PageSize:  pageSize,
		Search:    q.Search,
		IsActive:  q.IsActive,
		SortBy:    q.SortBy,
		SortOrder: q.SortOrder,
	}

	items, total, err := h.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if pageSize > 0 && total > 0 {
		computed := (total + int64(pageSize) - 1) / int64(pageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(page),
		PageSize:    safeconv.IntToInt32(pageSize),
	}, nil
}
