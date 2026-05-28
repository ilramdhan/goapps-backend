// Package datasource provides application-layer handlers for BI data source registry queries.
//
// In MVP this is read-only: List returns the registry; GetFactDistincts powers the admin form
// dropdowns and is cached upstream (5-min Redis TTL).
package datasource

import (
	"context"

	dsdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/datasource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ListHandler returns the data-source registry.
type ListHandler struct{ repo dsdomain.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r dsdomain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle returns active (or all if includeInactive) sources.
func (h *ListHandler) Handle(ctx context.Context, includeInactive bool) ([]*dsdomain.DataSource, error) {
	return h.repo.List(ctx, includeInactive)
}

// GetDistinctsQuery is the payload for GetDistinctsHandler.
type GetDistinctsQuery struct {
	Type string // optional scope; "" returns just the type list
}

// GetDistinctsHandler returns the distinct type/group_1/group_2/group_3 values for
// admin-form dropdowns. Callers should wrap with a 5-minute Redis cache.
type GetDistinctsHandler struct{ fact factmetric.Repository }

// NewGetDistinctsHandler constructs a GetDistinctsHandler.
func NewGetDistinctsHandler(f factmetric.Repository) *GetDistinctsHandler {
	return &GetDistinctsHandler{fact: f}
}

// Handle returns the distinct values.
func (h *GetDistinctsHandler) Handle(ctx context.Context, q GetDistinctsQuery) (factmetric.DistinctValues, error) {
	return h.fact.GetDistincts(ctx, factmetric.DistinctScope{Type: q.Type})
}
