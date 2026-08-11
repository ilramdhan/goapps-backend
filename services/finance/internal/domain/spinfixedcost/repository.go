// Package spinfixedcost provides domain logic for the POY spinning fixed-cost pool master.
package spinfixedcost

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the Spin Fixed Cost domain.
type Repository interface {
	// Create persists a new record.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves a record by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByPeriod retrieves the live record for a YYYYMM period.
	GetByPeriod(ctx context.Context, period string) (*Entity, error)

	// List retrieves records with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing record.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks a record as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByPeriod reports whether a live record exists for the given period.
	ExistsByPeriod(ctx context.Context, period string) (bool, error)

	// ExistsByID reports whether a live record exists for the given UUID.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// LoadAnchorStats gathers the counts the anchor-row guard needs, ignoring the
	// row identified by excludeID (the row about to be removed or deactivated).
	LoadAnchorStats(ctx context.Context, excludeID uuid.UUID) (AnchorStats, error)
}

// AnchorStats summarizes the surviving live rows used by CheckAnchorGuard.
// Every field is computed with the candidate row EXCLUDED, i.e. it describes the
// state the table would be left in.
type AnchorStats struct {
	// RemainingActiveCount is the number of live+active rows other than the candidate.
	RemainingActiveCount int64
	// EarliestRemainingActivePeriod is the smallest YYYYMM among those rows ("" if none).
	EarliestRemainingActivePeriod string
	// HasLiveRowAfterCandidate reports whether any live row (active or not) sits at a
	// period strictly later than the candidate's.
	HasLiveRowAfterCandidate bool
}

// CheckAnchorGuard refuses removals and deactivations that would silently zero POY
// fixed cost.
//
// The calc engine's LoadSpinFixedCost resolves a period with
//
//	WHERE msfc_is_active = TRUE AND deleted_at IS NULL AND msfc_period <= $1
//	ORDER BY msfc_period DESC LIMIT 1
//
// and returns an EMPTY MAP — not an error — when nothing matches. Downstream, the
// pool arm's divide guards turn that into a plain 0. So dropping the earliest
// live+active row does not fail anywhere: it just costs ~4,003 POY products at zero
// fixed cost, with every number involved still looking plausible. That is why this
// guard is a hard refusal rather than a warning.
//
// The rule: refuse when the candidate is the ONLY live+active row, and refuse when it
// is the earliest live+active row while later live rows still exist (those later rows
// would then have no anchor at or before the periods that currently resolve to it).
func CheckAnchorGuard(candidate *Entity, stats AnchorStats) error {
	// A row that is already inactive or deleted anchors nothing; removing it is safe.
	if !candidate.IsActive() || candidate.IsDeleted() {
		return nil
	}
	if stats.RemainingActiveCount == 0 {
		return ErrAnchorRowOnly
	}
	isEarliest := stats.EarliestRemainingActivePeriod == "" ||
		candidate.Period() < stats.EarliestRemainingActivePeriod
	if isEarliest && stats.HasLiveRowAfterCandidate {
		return ErrAnchorRowEarliest
	}
	return nil
}

// ListFilter contains filtering options for listing Spin Fixed Cost records.
type ListFilter struct {
	Search    string
	Period    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "period", "created_at", "updated_at"
	SortOrder string // "asc", "desc"
}

// Validate normalizes the filter to safe defaults.
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

// Offset returns the query offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
