// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence interface for MB Head.
type Repository interface {
	// Create persists a new MB Head.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves an MB Head by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByMBCosting retrieves an MB Head by its unique mb_costing value.
	GetByMBCosting(ctx context.Context, mbCosting string) (*Entity, error)

	// List retrieves MB Heads with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing MB Head.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks an MB Head as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByMBCosting checks if an MB Head with the given mb_costing exists.
	ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error)

	// ExistsByID checks if an MB Head with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ExistsByDevCode reports whether a live MB Head already uses the given development code.
	// excludeID, when non-nil, omits that row so an update does not collide with itself
	// (spec section 3.2).
	ExistsByDevCode(ctx context.Context, code string, excludeID *uuid.UUID) (bool, error)

	// ExistsByVsNumber reports whether a live MB Head already uses the given VS number.
	// excludeID, when non-nil, omits that row so an update does not collide with itself
	// (spec section 3.2).
	ExistsByVsNumber(ctx context.Context, number string, excludeID *uuid.UUID) (bool, error)

	// ListShades retrieves the live additional shades for one MB head, ordered by seq no.
	ListShades(ctx context.Context, mbhID uuid.UUID) ([]*Shade, error)

	// ListShadesByHeads retrieves the live additional shades for many MB heads at once, keyed by
	// head ID. It exists so the export can render the shade 2/3 columns without issuing one
	// ListShades per row across the whole table.
	ListShadesByHeads(ctx context.Context, mbhIDs []uuid.UUID) (map[uuid.UUID][]*Shade, error)

	// ReplaceShades reconciles the child shade rows for one MB head to exactly the supplied
	// set: rows absent from shades are soft-deleted, new rows inserted, existing rows updated.
	// An empty slice clears all children (spec section 4.4). Runs in a single transaction.
	ReplaceShades(ctx context.Context, mbhID uuid.UUID, shades []*Shade, actorUserID string) error

	// ListAll retrieves all non-deleted MB Heads matching filter, unpaginated (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*Entity, error)

	// UpdateEntryStatus persists a state-machine transition (entry_status, current_version, state_reason).
	UpdateEntryStatus(ctx context.Context, id uuid.UUID, entryStatus string, currentVersion int32, stateReason string) error

	// Transition atomically persists a workflow-state change: updates mst_mb_head's
	// entry_status/current_version/state_reason (and, when params is non-nil, the frozen
	// mbh_param_* snapshot columns), inserts a mst_mb_workflow_log audit row, and — only when
	// toState is StatusValidated — snapshots the current composition into
	// mst_mb_composition_version. All writes commit or roll back together.
	Transition(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *ParamSnapshot) error

	// TransitionWithAutoGen performs the same work as Transition, then — only when toState is
	// StatusValidated and entity's cost product has not already been generated — auto-generates
	// the linked cost_product_master/cost_route_*/CAPP/CPP rows per PRD §8.2 and writes back
	// mbh_cost_product_id/mbh_cost_generated_at/mbh_cost_generated_by onto mst_mb_head. All
	// writes (transition + auto-gen) commit or roll back together.
	TransitionWithAutoGen(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *ParamSnapshot, entity *Entity) error

	// RefreezeCostParams updates the frozen mbh_param_* columns on mst_mb_head and re-runs the
	// CPP (cost_product_parameter) freeze from the entity's current in-memory param getters.
	// Unlike Validate, this does not change entry_status, does not bump the version, does not
	// create a workflow-log row, and does not attempt auto-gen — it assumes the cost product
	// already exists. Safe to run against already-VALIDATED heads whose frozen values were
	// incorrect (e.g. after ENG-MB-01's fix for the throughput/no_of_process default bug).
	RefreezeCostParams(ctx context.Context, id uuid.UUID, entity *Entity, params *ParamSnapshot) error
}

// ParamSnapshot carries the 8 frozen recipe-parameter values for a VALIDATE transition. Nil
// when the transition is not VALIDATE (Submit/Approve/UnApprove/Revoke pass nil).
type ParamSnapshot struct {
	Waste, QualityLoss, Efficiency, DevExpense, Packing, MBProdPerDay *string
	ThroughputPerHour, NoOfProcess                                    string
}

// ListFilter contains filtering options for listing MB Heads.
type ListFilter struct {
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "mbh_mb_costing", "mbh_mgt_name", "mbh_denier", "created_at"
	SortOrder string // "asc", "desc"
}

// Validate normalizes filter values to safe defaults.
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
		f.SortBy = "mbh_mb_costing"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}

// ExportFilter contains filtering options for exporting MB Heads.
type ExportFilter struct {
	IsActive *bool
}
