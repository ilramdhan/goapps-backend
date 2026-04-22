// Package rmcost provides the landed-cost calculation engine and persistence contract
// for the RM cost aggregates produced from grouped raw-material consumption data.
package rmcost

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for `cst_rm_cost` and the
// append-only history table `aud_rm_cost_history`.
type Repository interface {
	// Upsert writes the Cost row keyed on (period, rm_code). When a row already
	// exists it is overwritten with the supplied fields; otherwise a new row is
	// inserted. Implementations must perform the write inside a transaction and
	// append one History row within the same transaction.
	Upsert(ctx context.Context, cost *Cost, hist History) error

	// GetByID retrieves a Cost by its primary key. Returns ErrNotFound when absent.
	GetByID(ctx context.Context, id uuid.UUID) (*Cost, error)

	// GetByPeriodAndCode retrieves a Cost by (period, rm_code). Returns ErrNotFound when absent.
	GetByPeriodAndCode(ctx context.Context, period, rmCode string) (*Cost, error)

	// List returns a page of Cost rows plus the total count of matching rows.
	List(ctx context.Context, filter ListFilter) ([]*Cost, int64, error)

	// ListAll returns every cost row matching the filter with no pagination,
	// ordered by period DESC then rm_code ASC. Used by export.
	ListAll(ctx context.Context, filter ExportFilter) ([]*Cost, error)

	// ListHistory returns the history rows matching the supplied filter, newest first.
	ListHistory(ctx context.Context, filter HistoryFilter) ([]History, int64, error)

	// ExistsForGroupHead reports whether any Cost row currently references the
	// given group head. Used as a delete-guard so a group head cannot be removed
	// after cost data has been generated for it.
	ExistsForGroupHead(ctx context.Context, groupHeadID uuid.UUID) (bool, error)

	// ListDistinctPeriods returns the set of periods that have at least one
	// cost row, ordered DESC (newest first).
	ListDistinctPeriods(ctx context.Context) ([]string, error)
}

// ListFilter describes pagination, search, and sort options for ListCosts.
type ListFilter struct {
	// Period matches the 6-character YYYYMM period exactly when non-empty.
	Period string

	// RMType filters by rm_type when non-empty.
	RMType RMType

	// GroupHeadID filters to costs belonging to the given group head when non-nil.
	GroupHeadID *uuid.UUID

	// Search matches against rm_code and rm_name.
	Search string

	Page     int
	PageSize int

	// SortBy accepts "period", "rm_code", "rm_name", "calculated_at".
	SortBy string
	// SortOrder accepts "asc" or "desc".
	SortOrder string
}

// NewListFilter returns a ListFilter with sensible defaults.
func NewListFilter() ListFilter {
	return ListFilter{
		Page:      1,
		PageSize:  10,
		SortBy:    "period",
		SortOrder: "desc",
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
		f.SortBy = "period"
	}
	if f.SortOrder == "" {
		f.SortOrder = "desc"
	}
}

// Offset returns the SQL OFFSET corresponding to Page/PageSize.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}

// ExportFilter scopes the unpaginated ListAll used by the Excel export path.
type ExportFilter struct {
	// Period matches the 6-character YYYYMM period exactly when non-empty.
	Period string
	// RMType filters by rm_type when non-empty.
	RMType RMType
	// GroupHeadID filters to costs belonging to the given group head when non-nil.
	GroupHeadID *uuid.UUID
	// Search matches against rm_code and rm_name.
	Search string
}

// HistoryFilter scopes a ListHistory query.
type HistoryFilter struct {
	// Period matches the 6-character YYYYMM period exactly when non-empty.
	Period string

	// RMCode matches a specific rm_code when non-empty.
	RMCode string

	// GroupHeadID filters to history rows for the given group head when non-nil.
	GroupHeadID *uuid.UUID

	// JobID filters to history rows produced by a given job run when non-nil.
	JobID *uuid.UUID

	Page     int
	PageSize int
}

// NewHistoryFilter returns a HistoryFilter with sensible defaults.
func NewHistoryFilter() HistoryFilter {
	return HistoryFilter{
		Page:     1,
		PageSize: 20,
	}
}

// Validate normalizes the filter so callers can pass partially-populated structs.
func (f *HistoryFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
}

// Offset returns the SQL OFFSET corresponding to Page/PageSize.
func (f *HistoryFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
