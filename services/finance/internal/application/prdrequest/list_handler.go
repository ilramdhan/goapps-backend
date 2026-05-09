// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery carries filter and pagination inputs to ListHandler.
type ListQuery struct {
	Search          string
	Status          string
	RequesterID     *uuid.UUID
	AssignedTo      *uuid.UUID
	RequesterDeptID *uuid.UUID
	SortField       string
	SortDesc        bool
	Page            int
	PageSize        int
}

// ListResult holds the paginated requests and metadata returned by ListHandler.
type ListResult struct {
	Requests    []*domainprdrequest.Request
	TotalItems  int
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler retrieves a paginated list of product requests.
type ListHandler struct {
	repo domainprdrequest.Repository
}

// NewListHandler constructs a ListHandler.
func NewListHandler(repo domainprdrequest.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes a paginated product request list query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := domainprdrequest.ListFilter(q)

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + filter.PageSize - 1) / filter.PageSize
		totalPages = safeconv.IntToInt32(computed)
	}

	return &ListResult{
		Requests:    items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
