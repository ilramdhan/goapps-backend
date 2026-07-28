// Package downtimereason provides domain logic for PPC downtime-reason master data.
package downtimereason

import "errors"

// Domain errors for downtime-reason operations.
var (
	// ErrNotFound is returned when a downtime reason is not found.
	ErrNotFound = errors.New("downtime reason not found")

	// ErrAlreadyExists is returned when a downtime reason with the same area and code already exists.
	ErrAlreadyExists = errors.New("downtime reason already exists")

	// ErrInvalidArea is returned when the area code is not one of TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")

	// ErrEmptyCode is returned when the code is empty.
	ErrEmptyCode = errors.New("downtime reason code cannot be empty")

	// ErrCodeTooLong is returned when the code exceeds max length.
	ErrCodeTooLong = errors.New("downtime reason code must be at most 20 characters")

	// ErrEmptyName is returned when the name is empty.
	ErrEmptyName = errors.New("downtime reason name cannot be empty")

	// ErrNameTooLong is returned when the name exceeds max length.
	ErrNameTooLong = errors.New("downtime reason name must be at most 100 characters")

	// ErrInvalidCategory is returned when the category is not one of the allowed values.
	ErrInvalidCategory = errors.New("invalid category: must be IDLE_POSITION, MACHINE_DOWN, or PRODUCTION_LOSS")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
