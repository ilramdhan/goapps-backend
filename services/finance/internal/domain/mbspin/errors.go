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
	// ErrTooManyChildren is returned when a recalc fan-out would exceed 20
	// children (largest observed head carries 15 spins); the caller is expected
	// to fall back to a manual recalc.
	ErrTooManyChildren = errors.New("mb spin has too many children for automatic recalc")

	// --- LDR adjustment / lock (Task E) ---

	// ErrLDRLockedActual is returned when SetLDRAdjustment is called while the
	// spin's LDR is locked as Actual. Unlock first via UnlockLDRActual.
	ErrLDRLockedActual = errors.New("mb spin ldr is locked as actual; unlock before changing adjustment")
)
