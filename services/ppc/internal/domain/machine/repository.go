// Package machine provides domain logic for PPC machine master data.
package machine

import (
	"context"
	"time"
)

// Repository defines persistence operations for machines.
type Repository interface {
	// GetByID retrieves a machine by its ID.
	GetByID(ctx context.Context, id int64) (*Machine, error)

	// List retrieves machines with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Machine, int64, error)

	// Update persists PPC-local changes to an existing machine.
	Update(ctx context.Context, entity *Machine) error

	// UpsertSourced merges a sync-sourced machine (from finance/Oracle),
	// preserving PPC-local fields (doff weight) and never overwriting existing
	// values with NULL. Reports whether the row was inserted, updated, or
	// skipped.
	UpsertSourced(ctx context.Context, src SourcedMachine) (UpsertOutcome, error)

	// EnsureGroup returns the id of the machine group with the given name and
	// area, creating it when absent. Used by the machine sync to materialize the
	// group set carried by Oracle TXTMACH.MACH_GROUP, which is the source of
	// truth for machine→group assignment.
	EnsureGroup(ctx context.Context, name, area string) (int64, error)
}

// SourcedMachine is a merged machine projection assembled from sync sources
// (finance mst_machine + Oracle TXTMACH) ahead of an UpsertSourced.
type SourcedMachine struct {
	// MachineNo is the natural key (finance mc_code / Oracle MACH_NO).
	MachineNo string
	// Area is TXT/SPG/TWT when resolved from Oracle MACH_DEPT; empty when only
	// finance knows the machine (area is then PPC-local and preserved).
	Area string
	// SourceMcID is the finance mst_machine UUID; nil for Oracle-only rows.
	SourceMcID *string
	// OrionCode is Oracle TXTMACH.MACH_ORION; empty when Oracle has no match
	// (the existing value is then preserved).
	OrionCode string
	// Line is Oracle TXTMACH.MACH_LINE; empty when unknown (preserved).
	Line string
	// GroupID is the machine_group resolved from TXTMACH.MACH_GROUP; nil when
	// Oracle carries no group for the machine (the existing value is preserved).
	GroupID *int64
	// IsActive is the source active flag.
	IsActive bool
	// SyncedAt is the sync timestamp stamped on the row.
	SyncedAt time.Time
}

// UpsertOutcome reports the result of an UpsertSourced merge.
type UpsertOutcome int

// UpsertSourced outcomes.
const (
	// OutcomeSkipped means no row was written (e.g. a new machine with no
	// resolvable area, which the NOT NULL area column cannot accept).
	OutcomeSkipped UpsertOutcome = iota
	// OutcomeInserted means a new machine row was created.
	OutcomeInserted
	// OutcomeUpdated means an existing machine row was merged.
	OutcomeUpdated
)

// ListFilter contains filtering and pagination options for listing machines.
type ListFilter struct {
	Search         string
	Area           string
	MachineGroupID *int64
	IsActive       *bool
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
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
		f.SortBy = "machine_no"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
