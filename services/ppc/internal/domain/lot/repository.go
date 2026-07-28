// Package lot provides domain logic for PPC lot-master data.
package lot

import "context"

// Repository defines persistence operations for lot masters.
type Repository interface {
	// Create persists a new lot master.
	Create(ctx context.Context, entity *Master) error

	// GetByID retrieves a lot master by its lot number.
	GetByID(ctx context.Context, lotNo string) (*Master, error)

	// List retrieves lot masters with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Master, int64, error)

	// Update persists changes to an existing lot master.
	Update(ctx context.Context, entity *Master) error

	// Delete removes a lot master by its lot number.
	Delete(ctx context.Context, lotNo string) error

	// UpsertSourcedBatch merges sync-sourced lots (from Oracle MMSMERGE) in bulk,
	// preserving PPC-local fields and never overwriting an existing value with
	// NULL. MMSMERGE carries ~66k rows, so the merge has to be set-based: one
	// round-trip per row overran the 60s RPC budget and left the import partial.
	//
	// Callers must pass at most one entry per LotNo — a single statement cannot
	// touch the same conflict target twice.
	UpsertSourcedBatch(ctx context.Context, src []SourcedLot) (UpsertBatchResult, error)
}

// ListFilter contains filtering and pagination options for listing lot masters.
type ListFilter struct {
	Search    string
	ItemCode  string
	ShadeCode string
	// Source filters on provenance (SourcePPC / SourceMMSMERGE); empty = all.
	Source string
	// ProdType filters on the MMSMERGE product type (PTY/POY/FOY); empty = all.
	ProdType  string
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
		f.SortBy = "lot_no"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
