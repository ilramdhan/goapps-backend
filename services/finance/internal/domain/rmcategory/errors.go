// Package rmcategory provides domain logic for Raw Material Category management.
package rmcategory

import "errors"

// Domain errors for RMCategory operations.
var (
	// ErrNotFound is returned when a raw material category is not found.
	ErrNotFound = errors.New("raw material category not found")

	// ErrAlreadyExists is returned when attempting to create a category with an existing code.
	ErrAlreadyExists = errors.New("raw material category already exists")

	// ErrEmptyCode is returned when the category code is empty.
	ErrEmptyCode = errors.New("category code cannot be empty")

	// ErrInvalidCodeFormat is returned when the category code format is invalid.
	ErrInvalidCodeFormat = errors.New("category code must start with uppercase letter and contain only uppercase letters, numbers, and underscores")

	// ErrCodeTooLong is returned when the category code exceeds max length.
	ErrCodeTooLong = errors.New("category code must be at most 20 characters")

	// ErrEmptyName is returned when the category name is empty.
	ErrEmptyName = errors.New("category name cannot be empty")

	// ErrNameTooLong is returned when the category name exceeds max length.
	ErrNameTooLong = errors.New("category name must be at most 100 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrEmptyUpdatedBy is returned when updated_by is empty.
	ErrEmptyUpdatedBy = errors.New("updated_by cannot be empty")

	// ErrAlreadyDeleted is returned when attempting to modify an already deleted category.
	ErrAlreadyDeleted = errors.New("raw material category is already deleted")
)
