// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// ListQuery represents the list users query.
type ListQuery struct {
	Page         int
	PageSize     int
	Search       string
	IsActive     *bool
	SectionID    *uuid.UUID
	DepartmentID *uuid.UUID
	DivisionID   *uuid.UUID
	CompanyID    *uuid.UUID
	SortBy       string
	SortOrder    string
}

// ListResult represents the list users result.
type ListResult struct {
	Users       []*user.WithDetail
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the list users query.
type ListHandler struct {
	repo user.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo user.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list users query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	// Build params.
	params := user.ListParams{
		Page:         query.Page,
		PageSize:     query.PageSize,
		Search:       query.Search,
		IsActive:     query.IsActive,
		SectionID:    query.SectionID,
		DepartmentID: query.DepartmentID,
		DivisionID:   query.DivisionID,
		CompanyID:    query.CompanyID,
		SortBy:       query.SortBy,
		SortOrder:    query.SortOrder,
	}

	// Default pagination.
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 10
	}

	// Execute query.
	users, total, err := h.repo.ListWithDetails(ctx, params)
	if err != nil {
		return nil, err
	}

	// Calculate total pages using safe conversion.
	var totalPages int32
	if params.PageSize > 0 && total > 0 {
		computed := (total + int64(params.PageSize) - 1) / int64(params.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Users:       users,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(params.Page),
		PageSize:    safeconv.IntToInt32(params.PageSize),
	}, nil
}
