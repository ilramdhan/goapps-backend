// Package boxbobbincost provides domain logic for Box Bobbin Cost management.
package boxbobbincost

import "errors"

// Domain errors for Box Bobbin Cost operations.
var (
	// ErrNotFound is returned when a box bobbin cost record is not found.
	ErrNotFound = errors.New("box bobbin cost not found")

	// ErrAlreadyExists is returned when attempting to create a record with an existing code.
	ErrAlreadyExists = errors.New("box bobbin cost code already exists")

	// ErrDuplicatePeriod is returned when attempting to add a rate for an already-used period.
	ErrDuplicatePeriod = errors.New("a rate for this period already exists")

	// ErrEmptyCode is returned when the code is empty.
	ErrEmptyCode = errors.New("box bobbin cost code cannot be empty")

	// ErrCodeTooLong is returned when the code exceeds 30 characters.
	ErrCodeTooLong = errors.New("box bobbin cost code must be at most 30 characters")

	// ErrEmptyName is returned when the name is empty.
	ErrEmptyName = errors.New("box bobbin cost name cannot be empty")

	// ErrNameTooLong is returned when the name exceeds 100 characters.
	ErrNameTooLong = errors.New("box bobbin cost name must be at most 100 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrAlreadyDeleted is returned when attempting to modify an already deleted record.
	ErrAlreadyDeleted = errors.New("box bobbin cost is already deleted")
)
