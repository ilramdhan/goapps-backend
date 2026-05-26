package dashboard

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the contract the infrastructure layer must implement for the
// Dashboard aggregate. All methods accept ctx (cancellation, tracing) and
// return wrapped sentinel errors from errors.go for classification.
type Repository interface {
	// Create persists a new dashboard. Returns ErrAlreadyExists on dashboard_code conflict.
	Create(ctx context.Context, d *Dashboard) error

	// GetByID hydrates a dashboard by primary key. Returns ErrNotFound on miss.
	GetByID(ctx context.Context, id uuid.UUID) (*Dashboard, error)

	// GetByCode hydrates by dashboard_code. Returns ErrNotFound on miss.
	GetByCode(ctx context.Context, code string) (*Dashboard, error)

	// List returns paginated dashboards matching the filter, along with the total count.
	List(ctx context.Context, f ListFilter) ([]*Dashboard, int64, error)

	// Update overwrites mutable fields (caller must have applied entity.Update beforehand).
	Update(ctx context.Context, d *Dashboard) error

	// SoftDelete marks the row as deleted (sets deleted_at + deleted_by + is_active=false).
	SoftDelete(ctx context.Context, id uuid.UUID, by uuid.UUID) error

	// Duplicate clones the dashboard with a fresh ID, the given new code/title, and a fresh role mapping.
	// Audit columns reset to created_at=NOW, created_by=by. Returns ErrAlreadyExists on conflict.
	Duplicate(ctx context.Context, sourceID uuid.UUID, newCode, newTitle string, by uuid.UUID) (*Dashboard, error)

	// SetRoles overwrites the per-dashboard role whitelist (delete-then-insert in TX).
	SetRoles(ctx context.Context, dashboardID uuid.UUID, roleCodes []string, by uuid.UUID) error

	// GetRoles returns the current role whitelist for a dashboard.
	GetRoles(ctx context.Context, dashboardID uuid.UUID) ([]string, error)

	// ListAccessible returns all active dashboards visible to the calling user.
	// Visibility rule: row has no entries in bi_dashboard_role OR user's role(s) intersect that set,
	// OR isSuperAdmin is true. Used by viewer sidebar.
	ListAccessible(ctx context.Context, userRoles []string, isSuperAdmin bool) ([]*Dashboard, error)
}

// ListFilter is the parameter object for Repository.List.
type ListFilter struct {
	Search          string     // substring match on dashboard_code or dashboard_title
	GroupID         *uuid.UUID // optional filter
	FilterType      string     // optional filter on bi_fact_metric.type slice
	IncludeInactive bool
	Page            int    // 1-based
	PageSize        int    // 1..100
	SortField       string // "code"|"title"|"display_order"|"created_at" (default display_order)
	SortDir         string // "asc"|"desc" (default asc)
}
