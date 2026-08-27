package mbcomposition

import "errors"

// Domain errors for MB composition operations.
var (
	// ErrMbhIDRequired is returned when mbh_id is empty.
	ErrMbhIDRequired = errors.New("mbcomposition: mbh_id is required")
	// ErrInvalidSourceType is returned when source_type is not one of the known constants.
	ErrInvalidSourceType = errors.New("mbcomposition: source_type must be GROUP, MB, or CARRIER")
	// ErrGroupHeadIDRequired is returned when source_type is GROUP but group_head_id is empty.
	ErrGroupHeadIDRequired = errors.New("mbcomposition: group_head_id is required when source_type is GROUP")
	// ErrCreatedByRequired is returned when created_by is empty.
	ErrCreatedByRequired = errors.New("mbcomposition: created_by is required")
	// ErrNotFound is returned when a composition row is not found.
	ErrNotFound = errors.New("mbcomposition: not found")
	// ErrParentHeadNotFound is returned when the parent mst_mb_head row a guarded
	// write refers to does not exist (or is soft-deleted). Only the guarded write
	// paths can report this: they must lock the parent row before reading its
	// composition total, so a missing parent is detected there rather than surfacing
	// later as a foreign-key violation.
	ErrParentHeadNotFound = errors.New("mbcomposition: parent mb head not found")
	// ErrParentNotDraft is returned when a composition write is attempted while the
	// parent mst_mb_head row is in any state other than DRAFT. The composition is the
	// working set of a recipe that is still being drafted; once the head leaves DRAFT
	// the lines it was validated/submitted with must stay exactly as they were, so the
	// gate is a WHITELIST (only DRAFT passes) rather than a blacklist of known-bad
	// states — a status added later is then refused by default instead of silently
	// opening a new hole.
	ErrParentNotDraft = errors.New("mbcomposition: composition can only be modified while the parent mb head is in DRAFT status")
)
