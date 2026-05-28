package upload

import (
	"context"
	"fmt"

	uploaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
)

// ListQuery is the input to ListHandler.
type ListQuery struct {
	Page     int
	PageSize int
}

// ListResult bundles a page of sessions with the total count.
type ListResult struct {
	Items []*uploaddomain.Upload
	Total int
}

// ListHandler returns paginated upload session history.
type ListHandler struct {
	repo uploaddomain.Repository
}

// NewListHandler constructs a ListHandler.
func NewListHandler(repo uploaddomain.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle returns a page of upload sessions.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.ListSessions(ctx, q.Page, q.PageSize)
	if err != nil {
		return ListResult{}, fmt.Errorf("list upload sessions: %w", err)
	}
	return ListResult{Items: items, Total: total}, nil
}
