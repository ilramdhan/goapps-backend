// Package machinegroup provides domain logic for PPC machine-group master data.
package machinegroup

import "errors"

// Domain errors for machine-group operations.
var (
	// ErrNotFound is returned when a machine group is not found.
	ErrNotFound = errors.New("machine group not found")

	// ErrAlreadyExists is returned when a machine group with the same name and area already exists.
	ErrAlreadyExists = errors.New("machine group already exists")

	// ErrEmptyName is returned when the group name is empty.
	ErrEmptyName = errors.New("machine group name cannot be empty")

	// ErrNameTooLong is returned when the group name exceeds max length.
	ErrNameTooLong = errors.New("machine group name must be at most 50 characters")

	// ErrInvalidArea is returned when the area code is not one of TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
