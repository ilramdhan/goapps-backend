// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// ListQuery represents the list roles query.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	IsSystem  *bool
	SortBy    string
	SortOrder string
}

// ListResult represents the list roles result.
type ListResult struct {
	Roles       []*role.Role
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListRoles query.
type ListHandler struct {
	repo role.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo role.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list roles query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	// Build params
	params := role.ListParams{
		Page:      query.Page,
		PageSize:  query.PageSize,
		Search:    query.Search,
		IsActive:  query.IsActive,
		IsSystem:  query.IsSystem,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// Apply defaults
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 10
	}

	// Execute query
	roles, total, err := h.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	// Calculate total pages
	var totalPages int32
	if params.PageSize > 0 && total > 0 {
		computed := (total + int64(params.PageSize) - 1) / int64(params.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Roles:       roles,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(params.Page),
		PageSize:    safeconv.IntToInt32(params.PageSize),
	}, nil
}
