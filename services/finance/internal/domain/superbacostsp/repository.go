package superbacostsp

import (
	"context"
	"time"
)

// Repository defines persistence operations for the Superba Cost SP master.
type Repository interface {
	// Create persists a hand-authored row and assigns its ID.
	Create(ctx context.Context, entity *Entry) error
	// GetByID retrieves a non-deleted row by id.
	GetByID(ctx context.Context, id string) (*Entry, error)
	// GetByLegacySysID retrieves a non-deleted row by legacy sys id.
	GetByLegacySysID(ctx context.Context, legacySysID int64) (*Entry, error)
	// List retrieves non-deleted rows with filtering and pagination, plus the
	// most recent ORACLE-sourced update time (nil if never synced).
	List(ctx context.Context, filter ListFilter) ([]*Entry, int64, *time.Time, error)
	// Update persists changes to an existing row.
	Update(ctx context.Context, entity *Entry) error
	// SoftDelete marks a row deleted.
	SoftDelete(ctx context.Context, id, deletedBy string) error
	// UpsertByLegacySysID writes one sourced row keyed on legacy sys id. Existing
	// rows (any provenance, including MANUAL, and soft-deleted ones) are overwritten.
	UpsertByLegacySysID(ctx context.Context, src Sourced) (UpsertOutcome, error)
	// ResolveByShades returns, per normalized shade, the effective active row
	// (duplicate shade -> max legacy_sys_id). Missing shades are absent.
	ResolveByShades(ctx context.Context, shades []string) (map[string]Resolved, error)
}

// Resolved is the effective master value for one normalized shade.
type Resolved struct {
	OldValue    float64
	ColourName  string
	LegacySysID int64
}

// UpsertOutcome reports what an upsert did to one row.
type UpsertOutcome int

// Upsert outcomes.
const (
	// OutcomeSkipped means the row was not written (e.g. empty shade).
	OutcomeSkipped UpsertOutcome = iota
	// OutcomeInserted means a new row was created.
	OutcomeInserted
	// OutcomeUpdated means an existing row changed.
	OutcomeUpdated
	// OutcomeUnchanged means an existing row already matched the source.
	OutcomeUnchanged
)

// Sourced is one row as read from the Oracle legacy source (transport struct).
type Sourced struct {
	LegacySysID int64
	ShadeCode   string
	ColourName  *string
	OldValue    float64
	NewValue    *float64
}

// Source reads Superba Cost SP rows from the Oracle legacy system. Nil when
// unconfigured, in which case sync reports ErrSyncNotConfigured.
type Source interface {
	ListSuperbaCostSP(ctx context.Context) ([]Sourced, error)
}

// ListFilter contains filtering and pagination options.
type ListFilter struct {
	Search       string
	IsActive     *bool
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
		f.SortBy = "shade_code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset.
func (f *ListFilter) Offset() int { return (f.Page - 1) * f.PageSize }
