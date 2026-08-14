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
)

// Required-field errors for the 11 mandatory recipe fields (spec section 2.1).
var (
	// ErrEmptyMgtName is returned when mbh_mgt_name (MB Name) is empty.
	ErrEmptyMgtName = errors.New("mb head mb name cannot be empty")
	// ErrMgtNameTooLong is returned when mbh_mgt_name exceeds 100 characters.
	ErrMgtNameTooLong = errors.New("mb head mb name must be at most 100 characters")

	// ErrEmptyDevCode is returned when mbh_dev_code (Development No) is empty.
	ErrEmptyDevCode = errors.New("mb head development no cannot be empty")
	// ErrDevCodeTooLong is returned when mbh_dev_code exceeds 50 characters.
	ErrDevCodeTooLong = errors.New("mb head development no must be at most 50 characters")

	// ErrEmptyVsNumber is returned when mbh_vs_number is empty.
	ErrEmptyVsNumber = errors.New("mb head vs number cannot be empty")
	// ErrVsNumberTooLong is returned when mbh_vs_number exceeds 50 characters.
	ErrVsNumberTooLong = errors.New("mb head vs number must be at most 50 characters")

	// ErrEmptyNoOfProcess is returned when mbh_no_of_process is empty.
	ErrEmptyNoOfProcess = errors.New("mb head no of process cannot be empty")
	// ErrNoOfProcessTooLong is returned when mbh_no_of_process exceeds 10 characters.
	ErrNoOfProcessTooLong = errors.New("mb head no of process must be at most 10 characters")
	// ErrInvalidNoOfProcess is returned when mbh_no_of_process is not one of the NO_OF_PROCESS
	// options configured in mst_mb_param_option (spec section 2.3).
	ErrInvalidNoOfProcess = errors.New("mb head no of process must be one of the configured options")

	// ErrEmptyShadeCode is returned when a shade code is empty.
	ErrEmptyShadeCode = errors.New("mb head shade code cannot be empty")
	// ErrShadeCodeTooLong is returned when a shade code exceeds 20 characters.
	ErrShadeCodeTooLong = errors.New("mb head shade code must be at most 20 characters")

	// ErrEmptyShadeName is returned when a shade name is empty.
	ErrEmptyShadeName = errors.New("mb head shade name cannot be empty")
	// ErrShadeNameTooLong is returned when a shade name exceeds 100 characters.
	ErrShadeNameTooLong = errors.New("mb head shade name must be at most 100 characters")

	// ErrEmptyCrossSection is returned when mbh_cross_section is empty.
	ErrEmptyCrossSection = errors.New("mb head cross section cannot be empty")
	// ErrCrossSectionTooLong is returned when mbh_cross_section exceeds 20 characters.
	ErrCrossSectionTooLong = errors.New("mb head cross section must be at most 20 characters")

	// ErrEmptyFinalProduct is returned when mbh_final_product is empty.
	ErrEmptyFinalProduct = errors.New("mb head final product cannot be empty")
	// ErrFinalProductTooLong is returned when mbh_final_product exceeds 200 characters.
	ErrFinalProductTooLong = errors.New("mb head final product must be at most 200 characters")

	// ErrInvalidDenier is returned when mbh_denier is not greater than zero.
	ErrInvalidDenier = errors.New("mb head poy denier must be greater than 0")
	// ErrInvalidFilament is returned when mbh_filament is not greater than zero.
	ErrInvalidFilament = errors.New("mb head poy filament must be greater than 0")
	// ErrInvalidLdrPercent is returned when mbh_ldr_prsn falls outside 0-100.
	ErrInvalidLdrPercent = errors.New("mb head ldr % must be between 0 and 100")
)

// Uniqueness errors, raised by the application pre-check and by the 23505 constraint mapping
// (spec section 3.2).
var (
	// ErrDevCodeAlreadyExists is returned when another live MB head already uses the dev code.
	ErrDevCodeAlreadyExists = errors.New("mb head development no already exists")
	// ErrVsNumberAlreadyExists is returned when another live MB head already uses the vs number.
	ErrVsNumberAlreadyExists = errors.New("mb head vs number already exists")
)

// Child-shade errors for the max-3-shades rule (spec section 4.2).
var (
	// ErrTooManyShades is returned when more than 2 additional child shades are supplied.
	ErrTooManyShades = errors.New("mb head accepts at most 2 additional shades (3 in total)")
	// ErrDuplicateShadeCode is returned when two additional shades share the same shade code.
	ErrDuplicateShadeCode = errors.New("mb head additional shade codes must be distinct")
	// ErrShadeCodeMatchesHeader is returned when an additional shade repeats the header shade code.
	ErrShadeCodeMatchesHeader = errors.New("mb head additional shade code must differ from the header shade code")
	// ErrInvalidShadeSeqNo is returned when an additional shade's sequence number is not 2 or 3.
	ErrInvalidShadeSeqNo = errors.New("mb head additional shade seq no must be 2 or 3")
	// ErrDuplicateShadeSeqNo is returned when two additional shades share the same sequence number.
	ErrDuplicateShadeSeqNo = errors.New("mb head additional shade seq no must be distinct")
	// ErrShadeNotFound is returned when an additional shade row cannot be located.
	ErrShadeNotFound = errors.New("mb head additional shade not found")
)
