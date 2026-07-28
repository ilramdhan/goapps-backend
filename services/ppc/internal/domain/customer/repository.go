// Package customer provides domain logic for the PPC customer master.
package customer

import (
	"context"
	"time"
)

// Repository defines persistence operations for the customer master.
type Repository interface {
	// Create persists a hand-authored customer and assigns its ID.
	Create(ctx context.Context, entity *Customer) error
	// GetByID retrieves a customer by its ID.
	GetByID(ctx context.Context, id int64) (*Customer, error)
	// GetByCode retrieves a customer by its (normalized) code.
	GetByCode(ctx context.Context, code string) (*Customer, error)
	// List retrieves customers with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Customer, int64, error)
	// ListAll retrieves every customer matching a filter, unpaginated, for export.
	ListAll(ctx context.Context, filter ListFilter) ([]*Customer, error)
	// Update persists changes to an existing customer.
	Update(ctx context.Context, entity *Customer) error

	// UpsertSourced writes one Oracle-sourced row, keyed on customer code, and
	// reports whether it was inserted, updated or left untouched. Rows whose
	// provenance is MANUAL are never overwritten and report OutcomeSkipped.
	UpsertSourced(ctx context.Context, src Sourced) (UpsertOutcome, error)

	// ResolveCodes maps normalized customer codes to customer ids. Codes with no
	// match are simply absent from the result — the caller decides what that means.
	ResolveCodes(ctx context.Context, codes []string) (map[string]int64, error)
}

// UpsertOutcome reports what a sync upsert did to one row.
type UpsertOutcome int

// Upsert outcomes reported by UpsertSourced.
const (
	// OutcomeSkipped means the row was left untouched (unchanged, or MANUAL).
	OutcomeSkipped UpsertOutcome = iota
	// OutcomeInserted means a new row was created.
	OutcomeInserted
	// OutcomeUpdated means an existing row was refreshed from the source.
	OutcomeUpdated
)

// Sourced is one customer row as read from Oracle MGTDAT.OM_CUSTOMER. It is a
// transport struct, not an aggregate: the repository turns it into a row.
type Sourced struct {
	Code            string
	Name            string
	ShortName       *string
	TaxNo           *string
	ParentCode      *string
	IsActive        bool
	SourceCreatedAt *time.Time
	SourceUpdatedAt *time.Time
}

// Source reads customer rows from the Oracle ERP. Implemented by the Oracle
// client adapter; nil when Oracle is unconfigured, so the sync degrades to a no-op.
type Source interface {
	ListCustomers(ctx context.Context) ([]Sourced, error)
}

// ListFilter contains filtering and pagination options for listing customers.
type ListFilter struct {
	Search   string
	IsActive *bool
	// Source filters on provenance (ORACLE / MANUAL). Empty means both.
	Source    string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
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
