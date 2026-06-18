// Package intermingling provides domain logic for Intermingling cost lookup management.
package intermingling

import "errors"

// Domain errors for Intermingling operations.
var (
	// ErrNotFound is returned when an intermingling record is not found.
	ErrNotFound = errors.New("intermingling not found")
	// ErrAlreadyExists is returned when attempting to create a record with an existing code.
	ErrAlreadyExists = errors.New("intermingling code already exists")
	// ErrEmptyCode is returned when the code is empty.
	ErrEmptyCode = errors.New("intermingling code cannot be empty")
	// ErrCodeTooLong is returned when the code exceeds 20 characters.
	ErrCodeTooLong = errors.New("intermingling code must be at most 20 characters")
	// ErrEmptyName is returned when the name is empty.
	ErrEmptyName = errors.New("intermingling name cannot be empty")
	// ErrNameTooLong is returned when the name exceeds 100 characters.
	ErrNameTooLong = errors.New("intermingling name must be at most 100 characters")
	// ErrInvalidCost is returned when cost_per_kg is negative.
	ErrInvalidCost = errors.New("intermingling cost per kg must not be negative")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted record.
	ErrAlreadyDeleted = errors.New("intermingling is already deleted")
)
