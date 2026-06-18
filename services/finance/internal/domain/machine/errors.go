// Package machine provides domain logic for Machine master data management.
package machine

import "errors"

// Domain errors for Machine operations.
var (
	// ErrNotFound is returned when a machine is not found.
	ErrNotFound = errors.New("machine not found")
	// ErrAlreadyExists is returned when attempting to create a machine with an existing code.
	ErrAlreadyExists = errors.New("machine code already exists")
	// ErrEmptyCode is returned when the machine code is empty.
	ErrEmptyCode = errors.New("machine code cannot be empty")
	// ErrCodeTooLong is returned when the machine code exceeds 30 characters.
	ErrCodeTooLong = errors.New("machine code must be at most 30 characters")
	// ErrEmptyName is returned when the machine name is empty.
	ErrEmptyName = errors.New("machine name cannot be empty")
	// ErrNameTooLong is returned when the machine name exceeds 100 characters.
	ErrNameTooLong = errors.New("machine name must be at most 100 characters")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted machine.
	ErrAlreadyDeleted = errors.New("machine is already deleted")
)
