// Package shade provides domain logic for the shade master (cost_erp_shade).
package shade

import (
	"context"
	"time"
)

// Repository defines persistence operations for the shade master.
type Repository interface {
	// Create persists a hand-authored shade and assigns its ID.
	Create(ctx context.Context, entity *Shade) error
	// GetByID retrieves a shade by its ID.
	GetByID(ctx context.Context, id int64) (*Shade, error)
	// GetByCode retrieves a shade by its (normalized) code.
	GetByCode(ctx context.Context, code string) (*Shade, error)
	// List retrieves shades with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Shade, int64, error)
	// Update persists changes to an existing shade.
	Update(ctx context.Context, entity *Shade) error

	// UpsertSourced writes one Oracle-sourced row, keyed on shade code, and
	// reports whether it was inserted, updated or left untouched. Rows whose
	// provenance is MANUAL are never overwritten and report OutcomeSkipped.
	UpsertSourced(ctx context.Context, src Sourced) (UpsertOutcome, error)
}

// UpsertOutcome reports what a sync upsert did to one row.
type UpsertOutcome int

// Upsert outcomes reported by UpsertSourced.
const (
	// OutcomeSkipped means the row was left untouched (empty code, or MANUAL).
	OutcomeSkipped UpsertOutcome = iota
	// OutcomeInserted means a new row was created.
	OutcomeInserted
	// OutcomeUpdated means an existing row was refreshed from the source.
	OutcomeUpdated
)

// Sourced is one shade row as read from Oracle MGTDAT.OM_GRADE_CODE_2. It is a
// transport struct, not an aggregate: the repository turns it into a row.
type Sourced struct {
	Code            string
	Name            string
	ShortName       *string
	IsActive        bool
	SourceCreatedAt *time.Time
	SourceUpdatedAt *time.Time
	SourceCreatedBy *string
	SourceUpdatedBy *string
}

// Source reads shade rows from the Oracle ERP. Implemented by the Oracle client
// adapter; nil when Oracle is unconfigured, so the sync degrades to a no-op.
type Source interface {
	ListShades(ctx context.Context) ([]Sourced, error)
}

// ListFilter contains filtering and pagination options for listing shades.
type ListFilter struct {
	Search   string
	IsActive *bool
	// SourceFilter filters on provenance (ORACLE / MANUAL). Empty means both.
	SourceFilter string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// Validate normalizes pagination and sort defaults.
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

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int { return (f.Page - 1) * f.PageSize }
