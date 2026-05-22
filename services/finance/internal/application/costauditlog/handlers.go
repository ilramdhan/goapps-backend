// Package costauditlog holds audit-log read + emit use cases.
package costauditlog

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
)

// ListQuery is the read input.
type ListQuery struct {
	EntityType string
	EntityID   int64
	UserID     string
	Operation  string
	FromDate   string
	ToDate     string
	Page       int
	PageSize   int
}

// ListResult bundles.
type ListResult struct {
	Items []*domain.Log
	Total int64
}

// ListHandler returns paginated audit rows.
type ListHandler struct{ repo domain.Repository }

// NewListHandler constructs.
func NewListHandler(r domain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle lists.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, domain.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total}, nil
}

// Emitter is the helper used by business handlers to append audit rows.
// Construct one per service via NewEmitter and inject as a dependency.
type Emitter struct{ repo domain.Repository }

// NewEmitter wraps a repo for write-side use.
func NewEmitter(r domain.Repository) *Emitter { return &Emitter{repo: r} }

// Emit appends an audit row. Errors are returned to the caller — typically the
// caller logs and proceeds (audit failures should not block business operations).
func (e *Emitter) Emit(ctx context.Context, in domain.NewInput) error {
	return e.repo.Emit(ctx, in)
}
