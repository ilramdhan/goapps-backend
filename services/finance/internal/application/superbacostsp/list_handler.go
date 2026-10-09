package superbacostsp

import (
	"context"
	"time"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery is the list query.
type ListQuery struct {
	Page         int
	PageSize     int
	Search       string
	IsActive     *bool
	SourceFilter string
	SortBy       string
	SortOrder    string
}

// ListResult is the list result.
type ListResult struct {
	Items        []*domain.Entry
	TotalItems   int64
	TotalPages   int32
	CurrentPage  int32
	PageSize     int32
	LastSyncedAt *time.Time
}

// ListHandler handles ListSuperbaCostSps.
type ListHandler struct{ repo domain.Repository }

// NewListHandler creates a ListHandler.
func NewListHandler(repo domain.Repository) *ListHandler { return &ListHandler{repo: repo} }

// Handle executes the query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := domain.ListFilter{
		Search: q.Search, IsActive: q.IsActive, SourceFilter: q.SourceFilter,
		Page: q.Page, PageSize: q.PageSize, SortBy: q.SortBy, SortOrder: q.SortOrder,
	}
	filter.Validate()

	items, total, lastSynced, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	var totalPages int32
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}
	return &ListResult{
		Items: items, TotalItems: total, TotalPages: totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page), PageSize: safeconv.IntToInt32(filter.PageSize),
		LastSyncedAt: lastSynced,
	}, nil
}
