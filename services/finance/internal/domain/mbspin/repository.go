// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence interface for MB Spin.
type Repository interface {
	// Create persists a new MB Spin.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves an MB Spin by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// List retrieves MB Spins with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing MB Spin.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks an MB Spin as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByID checks if an MB Spin with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// GetByMBCosting retrieves an MB Spin by its MB costing code.
	GetByMBCosting(ctx context.Context, code string) (*Entity, error)

	// GetByOrionItemCode retrieves the first active MB Spin with the given ORION item code.
	// Returns ErrNotFound if no match exists.
	GetByOrionItemCode(ctx context.Context, code string) (*Entity, error)

	// DuplicateSpin transactionally clones one spin into a fresh R&D child that
	// records its origin in mbs_parent_spin_id. See DuplicateInput for exactly
	// which columns are copied, nulled, and set.
	//
	// Returns ErrNotFound when the source spin is absent or soft-deleted,
	// ErrParentCycle / ErrMaxDuplicateDepth when the source's lineage chain is
	// unusable.
	DuplicateSpin(ctx context.Context, in DuplicateInput) (DuplicateOutput, error)

	// ListChildren returns the direct recalc CANDIDATES of a parent spin:
	// mbs_parent_spin_id = parentID AND deleted_at IS NULL AND status = StatusRnD.
	//
	// ⚠ Deliberately NOT "all children". Non-R&D children (Spinning / Boughtout /
	// NULL status) are excluded here (A7) — the caller reports them as skipped
	// using its own query, and the caller must NOT recurse: recalc is ONE level
	// deep, grandchildren never participate (R13).
	ListChildren(ctx context.Context, parentID uuid.UUID) ([]*Entity, error)

	// ExistsByOrionItemCode reports whether any non-deleted spin already carries
	// this ORION item code.
	//
	// ⚠ Intended ONLY for checking a value that is CHANGING. Callers must skip
	// the check for empty codes and for unchanged codes: 177 legacy codes are
	// shared by 466 rows, and saving those rows must keep working. Because the
	// check runs only when the code changes, the row being saved can never be
	// its own match, so no self-exclusion parameter is needed.
	ExistsByOrionItemCode(ctx context.Context, code string) (bool, error)

	// ResolveUniqueByOrionItemCode looks up non-deleted spins by ORION item
	// code and reports whether EXACTLY ONE matched.
	//
	// ⚠ Unlike GetByOrionItemCode (read/display path, D20/M2a deterministic
	// tie-breaker via ORDER BY ... LIMIT 1), this method is for the SAVE-time
	// resolution of cpp_value_mb_spin_id and MUST NOT pick a winner among
	// duplicates: 177 legacy ORION codes are shared by up to 16 rows each
	// (466 rows total). ok=false (with a zero uuid.UUID and a nil error) means
	// "zero or more than one match" — the caller must leave the resolved
	// column NULL rather than guessing. A real error is returned only for a
	// genuine query failure.
	ResolveUniqueByOrionItemCode(ctx context.Context, code string) (id uuid.UUID, ok bool, err error)

	// ListByOrionItemCode returns every non-deleted mst_mb_spin row sharing
	// the given ORION item code — the full candidate set behind an ambiguous
	// MB_SPIN lookup value.
	//
	// ⚠ Uses the IDENTICAL matching rule as ResolveUniqueByOrionItemCode
	// (mbs_orion_item_code = code AND deleted_at IS NULL, no mbs_is_active
	// filter, no TRIM/UPPER normalization) so the candidate list shown to the
	// user never disagrees with what Upsert would resolve to on save.
	// Ordered by created_at, mbs_id for a stable, deterministic display order.
	ListByOrionItemCode(ctx context.Context, code string) ([]*Entity, error)
}

// MaxLineageDepth caps the mbs_parent_spin_id walk-up performed before a
// duplicate. A chain longer than this is treated as corrupt data rather than a
// legitimately deep lineage, so the walk can never run unbounded.
const MaxLineageDepth = 32

// MaxRecalcChildren caps the recalc fan-out from one parent. Beyond this the
// caller must fall back to a manual recalc (ErrTooManyChildren). The largest
// head observed in production carries 15 spins.
const MaxRecalcChildren = 20

// DuplicateInput is the payload for DuplicateSpin.
//
// Column policy, fixed by A5 / D19 — the persistence layer implements exactly this:
//
//	NULLED  : mbs_oracle_sys_id, mbs_orion_item_code, mbs_mb_costing
//	          (ERP/identity keys — a clone is not yet known to any ERP)
//	SET     : mbs_parent_spin_id = SourceSpinID, mbs_status = StatusRnD (D5),
//	          mbs_is_active = TRUE, mbs_duplicated_at/by,
//	          mbs_cost_product_id = the parent HEAD's mbh_cost_product_id (may be NULL),
//	          mbs_ldr_is_fixed = FALSE and mbs_dozing_is_fixed = FALSE — written
//	          EXPLICITLY, never left NULL, because NULL means "fixed" and would
//	          permanently exclude the clone from recalc.
//	COPIED  : mbs_mbh_id, mbs_mgt_name (+ " (copy)" unless overridden),
//	          mbs_denier, mbs_filament, mbs_dozing, mbs_cc, mbs_cost_rate_mkt,
//	          mbs_ldr_prsn, mbs_final_product, mbs_lesture
type DuplicateInput struct {
	// SourceSpinID is the spin to clone; it becomes the clone's mbs_parent_spin_id.
	SourceSpinID uuid.UUID
	// MgtName overrides the clone's name. nil => source name + " (copy)".
	MgtName *string
	// Denier overrides the copied mbs_denier. nil => copy the source value.
	Denier *float64
	// Filament overrides the copied mbs_filament. nil => copy the source value.
	Filament *int
	// ActorUserID lands in created_by and mbs_duplicated_by.
	ActorUserID string
}

// DuplicateOutput is the result of a successful DuplicateSpin.
type DuplicateOutput struct {
	// NewSpinID is the primary key of the freshly inserted clone.
	NewSpinID uuid.UUID
	// ParentSpinID echoes the source, i.e. the value stored in mbs_parent_spin_id.
	ParentSpinID uuid.UUID
	// HeadID is the mbs_mbh_id shared by source and clone.
	HeadID uuid.UUID
	// MgtName is the name actually stored (override, or source + " (copy)").
	MgtName string
	// LineageDepth is how many ancestors the pre-insert walk-up traversed. 0 for
	// a source that is not itself a clone.
	LineageDepth int
}

// ListFilter contains filtering options for listing MB Spins.
type ListFilter struct {
	HeadID    uuid.UUID
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "mbs_mgt_name", "mbs_denier", "created_at"
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
		f.SortBy = "mbs_mgt_name"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the pagination offset.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
