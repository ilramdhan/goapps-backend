// Package session provides application layer handlers for session management.
package session

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// ListQuery contains parameters for listing active sessions.
type ListQuery struct {
	Page        int
	PageSize    int
	Search      string
	ServiceName string
	UserID      *uuid.UUID
	SortBy      string
	SortOrder   string
}

// ListResult contains the result of listing sessions.
type ListResult struct {
	Sessions    []*session.Info
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles listing active sessions.
type ListHandler struct {
	repo session.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo session.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list sessions query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	params := session.ListParams{
		Page:        page,
		PageSize:    pageSize,
		Search:      query.Search,
		ServiceName: query.ServiceName,
		UserID:      query.UserID,
		SortBy:      query.SortBy,
		SortOrder:   query.SortOrder,
	}

	sessions, total, err := h.repo.ListActive(ctx, params)
	if err != nil {
		return nil, err
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &ListResult{
		Sessions:    sessions,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(page),
		PageSize:    safeconv.IntToInt32(pageSize),
	}, nil
}
