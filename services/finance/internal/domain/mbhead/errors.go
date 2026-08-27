// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import "errors"

// Domain errors for MB Head operations.
var (
	// ErrNotFound is returned when an MB head record is not found.
	ErrNotFound = errors.New("mb head not found")
	// ErrAlreadyExists is returned when attempting to create a record with an existing mb_costing.
	ErrAlreadyExists = errors.New("mb head mb_costing already exists")
	// ErrEmptyMBCosting is returned when mb_costing is empty.
	ErrEmptyMBCosting = errors.New("mb head mb_costing cannot be empty")
	// ErrMBCostingTooLong is returned when mb_costing exceeds 100 characters.
	ErrMBCostingTooLong = errors.New("mb head mb_costing must be at most 100 characters")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted MB head.
	ErrAlreadyDeleted = errors.New("mb head is already deleted")
	// ErrInvalidTransition is returned when a workflow state transition is not allowed.
	ErrInvalidTransition = errors.New("mbhead: invalid state transition")
	// ErrReasonRequired is returned when a transition requires a reason but none was given.
	ErrReasonRequired = errors.New("mbhead: reason is required for this transition")
	// ErrTooManyShades is returned when more than 2 additional shades are supplied
	// (header shade + 2 rows = 3 shades max per head, migration 000483).
	ErrTooManyShades = errors.New("mbhead: at most 2 additional shades are allowed")
	// ErrInvalidNoOfProcess is returned when mbh_no_of_process is not a known option code.
	ErrInvalidNoOfProcess = errors.New("mbhead: invalid number of process")
	// ErrInvalidCrossSection is returned when mbh_cross_section is not a known master value.
	ErrInvalidCrossSection = errors.New("mbhead: invalid cross section")
	// ErrDuplicateVSNumber is returned when a VS Number is already used by another MB head.
	ErrDuplicateVSNumber = errors.New("mbhead: vs number already exists")
	// ErrDuplicateDevCode is returned when a Dev Code is already used by another MB head.
	//
	// U-D (2026-08-22): Dev Code must be unique for NEW data, while pre-existing
	// legacy duplicates are left alone. Enforced exactly like ErrDuplicateVSNumber —
	// in the application layer, only when the value actually changes, and with ⛔ NO
	// UNIQUE constraint in the database (one would reject the legacy rows outright).
	ErrDuplicateDevCode = errors.New("mbhead: dev code already exists")
	// ErrRecipeFieldRequired is returned when a mandatory MB recipe field is empty.
	ErrRecipeFieldRequired = errors.New("mbhead: required recipe field is empty")
	// ErrHeadLocked is returned when an MB head is locked and cannot be modified.
	ErrHeadLocked = errors.New("mbhead: recipe is locked")
	// ErrUnlockNotRequested is returned when granting an unlock that was never requested.
	ErrUnlockNotRequested = errors.New("mbhead: no pending unlock request")
	// ErrUnlockOriginUnknown is returned when a pending unlock request is rejected but
	// the locked state it was parked from (APPROVED or VALIDATED) cannot be established.
	// ⛔ The domain refuses rather than guessing: returning a VALIDATED recipe as merely
	// APPROVED — or the reverse — would silently rewrite its costing standing.
	ErrUnlockOriginUnknown = errors.New("mbhead: cannot determine the state to return to after rejecting an unlock")
)
