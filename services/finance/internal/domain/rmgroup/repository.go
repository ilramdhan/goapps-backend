// Package rmgroup provides domain logic for raw-material grouping and landed-cost configuration.
package rmgroup

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for RM group heads and details.
// The interface lives in the domain layer; the implementation sits in infrastructure.
type Repository interface {
	// ---------- Head operations ----------

	// CreateHead persists a new Head row.
	CreateHead(ctx context.Context, head *Head) error

	// GetHeadByID retrieves a head by ID. Returns ErrNotFound when absent.
	GetHeadByID(ctx context.Context, id uuid.UUID) (*Head, error)

	// GetHeadByCode retrieves a head by its unique code. Returns ErrNotFound when absent.
	GetHeadByCode(ctx context.Context, code Code) (*Head, error)

	// ListHeads returns a page of heads plus the total count of matching rows.
	ListHeads(ctx context.Context, filter ListFilter) ([]*Head, int64, error)

	// ListAllHeads returns every non-deleted head matching the active filter.
	// Used by export — no pagination. activeFilter nil = all.
	ListAllHeads(ctx context.Context, activeFilter *bool) ([]*Head, error)

	// UpdateHead persists changes to an existing head.
	UpdateHead(ctx context.Context, head *Head) error

	// SoftDeleteHead marks the head and all of its active details as deleted.
	SoftDeleteHead(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsHeadByCode reports whether a non-deleted head with this code exists.
	ExistsHeadByCode(ctx context.Context, code Code) (bool, error)

	// ExistsHeadByID reports whether a non-deleted head with this ID exists.
	ExistsHeadByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ---------- Detail operations ----------

	// AddDetail persists a new Detail row.
	AddDetail(ctx context.Context, detail *Detail) error

	// UpdateDetail persists changes to an existing detail.
	UpdateDetail(ctx context.Context, detail *Detail) error

	// GetDetailByID retrieves a detail by ID.
	GetDetailByID(ctx context.Context, id uuid.UUID) (*Detail, error)

	// GetActiveDetailByItemCodeGrade looks up the active, non-deleted detail
	// owning the given (item_code, grade_code) pair across ALL groups. Used to
	// enforce the "one (item, grade) variant, one active group" invariant.
	// The Oracle sync feed keys items on (item_code, grade_code), so treating
	// the grade as part of the natural key keeps multi-variant items (same
	// item_code with different grades) independently groupable.
	// gradeCode may be empty — matches rows with NULL / empty grade_code.
	// Returns ErrDetailNotFound when the variant is not currently assigned.
	GetActiveDetailByItemCodeGrade(ctx context.Context, itemCode ItemCode, gradeCode string) (*Detail, error)

	// ListDetailsByHeadID returns every detail that belongs to the given head,
	// including soft-deleted rows (callers filter as needed). Ordered by sort_order.
	ListDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*Detail, error)

	// ListActiveDetailsByHeadID returns only the active, non-deleted details used by
	// the landed-cost engine for rate aggregation.
	ListActiveDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*Detail, error)

	// SoftDeleteDetail marks a single detail row as deleted.
	SoftDeleteDetail(ctx context.Context, id uuid.UUID, deletedBy string) error
}

// ListFilter describes pagination, search, and sort options for ListHeads.
type ListFilter struct {
	// Search matches against code, name, description, colourant, ci_name.
	Search string

	// IsActive filters by the is_active flag when non-nil.
	IsActive *bool

	// Flag filter — when non-empty, matches heads where ANY of the three flag_*
	// columns equals this value. Empty disables the filter.
	Flag Flag

	Page     int
	PageSize int

	// SortBy accepts "code", "name", "created_at", "updated_at".
	SortBy string
	// SortOrder accepts "asc" or "desc".
	SortOrder string
}

// NewListFilter returns a ListFilter with sensible defaults.
func NewListFilter() ListFilter {
	return ListFilter{
		Page:      1,
		PageSize:  10,
		SortBy:    "code",
		SortOrder: "asc",
	}
}

// Validate normalizes the filter so callers can pass partially-populated structs.
func (f *ListFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 10
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	if f.SortBy == "" {
		f.SortBy = "code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL OFFSET corresponding to Page/PageSize.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
