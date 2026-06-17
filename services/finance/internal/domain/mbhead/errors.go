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
)
