// Package productgrade provides domain logic for Product Grade management.
package productgrade

import "errors"

// Domain errors for Product Grade operations.
var (
	// ErrNotFound is returned when a product grade is not found.
	ErrNotFound = errors.New("product grade not found")

	// ErrAlreadyExists is returned when attempting to create a grade with an existing code.
	ErrAlreadyExists = errors.New("product grade code already exists")

	// ErrEmptyCode is returned when the grade code is empty.
	ErrEmptyCode = errors.New("product grade code cannot be empty")

	// ErrCodeTooLong is returned when the grade code exceeds max length.
	ErrCodeTooLong = errors.New("product grade code must be at most 10 characters")

	// ErrEmptyName is returned when the grade name is empty.
	ErrEmptyName = errors.New("product grade name cannot be empty")

	// ErrNameTooLong is returned when the grade name exceeds max length.
	ErrNameTooLong = errors.New("product grade name must be at most 100 characters")

	// ErrDescriptionTooLong is returned when description exceeds max length.
	ErrDescriptionTooLong = errors.New("product grade description must be at most 500 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrAlreadyDeleted is returned when attempting to modify an already deleted grade.
	ErrAlreadyDeleted = errors.New("product grade is already deleted")
)
