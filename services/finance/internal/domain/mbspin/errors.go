// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import "errors"

// Domain errors for MB Spin operations.
var (
	// ErrNotFound is returned when an MB spin record is not found.
	ErrNotFound = errors.New("mb spin not found")
	// ErrAlreadyExists is returned when the oracle_sys_id already exists.
	ErrAlreadyExists = errors.New("mb spin oracle_sys_id already exists")
	// ErrInvalidHeadID is returned when headID is nil UUID.
	ErrInvalidHeadID = errors.New("mb spin head_id cannot be nil")
	// ErrEmptyMgtName is returned when mgt_name is empty.
	ErrEmptyMgtName = errors.New("mb spin mgt_name cannot be empty")
	// ErrMgtNameTooLong is returned when mgt_name exceeds 100 characters.
	ErrMgtNameTooLong = errors.New("mb spin mgt_name must be at most 100 characters")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted spin.
	ErrAlreadyDeleted = errors.New("mb spin is already deleted")
	// ErrHeadNotFound is returned when the referenced MB head does not exist.
	ErrHeadNotFound = errors.New("mb head not found")

	// --- Duplicate / recalc (P8) ---

	// ErrParentCycle is returned when the mbs_parent_spin_id walk-up reaches the
	// spin being duplicated, i.e. the lineage chain already contains it. The DB
	// does NOT guard this: migration 000484 deliberately ships without the
	// chk_mbs_parent_not_self CHECK, so even a 1-hop self-loop is only prevented
	// here (R8/G8).
	ErrParentCycle = errors.New("mb spin parent lineage forms a cycle")
	// ErrMaxDuplicateDepth is returned when the lineage walk-up exceeds its
	// 32-hop budget without terminating. Treated as a fault rather than a deep
	// but legal chain: an unbounded walk on corrupt data must not run forever.
	ErrMaxDuplicateDepth = errors.New("mb spin parent lineage exceeds maximum depth")
	// ErrDuplicateOrionItemCode is returned when a NEW mbs_orion_item_code value
	// collides with an existing one. ⚠ Enforced only for values that CHANGE:
	// the 177 legacy duplicate codes (466 rows) must keep saving, and migration
	// 000486's unique index was permanently abandoned, so this is the sole
	// enforcer of the invariant.
	ErrDuplicateOrionItemCode = errors.New("mb spin orion_item_code already in use")
	// ErrAlreadyDuplicated is returned when the SOURCE spin passed to
	// DuplicateMBSpin already has a non-nil mbs_parent_spin_id, i.e. it is
	// itself the result of a prior duplicate. Only a root spin (mbs_parent_spin_id
	// IS NULL) may be duplicated: lineage depth is capped at one level and
	// grandchildren are no longer allowed, superseding the earlier multi-level
	// design.
	ErrAlreadyDuplicated = errors.New("mb spin has already been duplicated and cannot be duplicated again")
	// ErrTooManyChildren is returned when a recalc fan-out would exceed 20
	// children (largest observed head carries 15 spins); the caller is expected
	// to fall back to a manual recalc.
	ErrTooManyChildren = errors.New("mb spin has too many children for automatic recalc")

	// --- LDR adjustment / lock (Task E) ---

	// ErrLDRLockedActual is returned when SetLDRAdjustment is called while the
	// spin's LDR is locked as Actual. Unlock first via UnlockLDRActual.
	ErrLDRLockedActual = errors.New("mb spin ldr is locked as actual; unlock before changing adjustment")

	// --- Delete guards ---

	// ErrHasChildren is returned when attempting to delete a spin that still has
	// one or more live duplicated children (mbs_parent_spin_id = this spin's id
	// AND deleted_at IS NULL). Lineage integrity is enforced regardless of the
	// child's status: a child that has since moved past R&D still counts.
	ErrHasChildren = errors.New("mb spin cannot be deleted while it has duplicated children")

	// ErrInUse is returned when attempting to delete a spin that is still
	// referenced by a Master Product's cost parameter
	// (cost_product_parameter.cpp_value_mb_spin_id). The FK is ON DELETE SET
	// NULL but only fires on a hard delete, so this guard is required because
	// deletion here is a soft UPDATE.
	ErrInUse = errors.New("mb spin cannot be deleted while it is used by a master product's cost parameter")

	// --- FG (POY) link (mb-spin-fg-poy-link P2) ---
	//
	// Defined here (not in mbhead) because mbspin has no domain imports, so the
	// mbhead application layer can reuse them without an import cycle.

	// ErrFgNotPoy is returned when the chosen FG product is not an active, live POY product.
	ErrFgNotPoy = errors.New("fg product must be an active POY product")
	// ErrFgShadeMismatch is returned when the POY shade differs from the recipe/spin shade.
	ErrFgShadeMismatch = errors.New("fg poy shade does not match the recipe shade")
	// ErrSpinShadeFgExists is returned when a live spin already exists for the same shade + FG
	// (index uix_mbs_shade_fg_live).
	ErrSpinShadeFgExists = errors.New("a live spin for this shade and fg product already exists")
	// ErrFgFollowsRecipe is returned when a manual FG change targets a spin whose FG comes from its recipe.
	ErrFgFollowsRecipe = errors.New("fg of a recipe-generated spin follows its recipe; change it on the recipe")
	// ErrSpinSuperseded is returned (400) when a superseded spin is re-activated outside Un-supersede.
	ErrSpinSuperseded = errors.New("mb spin is superseded; use Un-supersede to restore it")
	// ErrFgSpinNotLive is returned when a manual FG change targets an inactive or superseded spin.
	ErrFgSpinNotLive = errors.New("fg can only be changed on a live (active, not superseded) mb spin")
	// ErrInvalidFgLinkSource is returned when fg_link_source is not RECIPE|DUPLICATE|MANUAL|BACKFILL.
	ErrInvalidFgLinkSource = errors.New("invalid fg link source")
	// ErrSupersedeConfirmationRequired is returned (409) when Validate would create a root
	// spin whose shade + FG collides with a live spin of another recipe and the request did
	// not confirm the supersede. The caller shows the supersede preview.
	ErrSupersedeConfirmationRequired = errors.New("a live spin of another recipe has the same shade and fg product; supersede confirmation required (see supersede-preview)")
)

// Un-supersede errors (mb-spin-fg-poy-link P3, design Revisi 1 D8).
var (
	// ErrNotSuperseded is returned (400) when un-supersede targets a spin that is not superseded.
	ErrNotSuperseded = errors.New("mb spin is not superseded")
	// ErrUnsupersedeNoBatch is returned (409) when no un-reverted supersede batch exists for the spin.
	ErrUnsupersedeNoBatch = errors.New("no un-reverted supersede batch found for this mb spin")
	// ErrUnsupersedeConflict is returned (409) when restoring the spin would collide on
	// shade + FG with another live spin (other than the one that superseded it).
	ErrUnsupersedeConflict = errors.New("cannot restore: another live spin has the same shade and fg product")
	// ErrInvalidActualLDR is returned when SetActualLDR receives a non-positive percentage.
	ErrInvalidActualLDR = errors.New("mb spin actual ldr percentage must be greater than zero")
)
