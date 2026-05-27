// Package uomcategory provides domain logic for UOM Category management.
package uomcategory

import "errors"

// Domain errors for UOM Category operations.
var (
	// ErrNotFound is returned when a UOM category is not found.
	ErrNotFound = errors.New("uom category not found")

	// ErrAlreadyExists is returned when attempting to create a category with an existing code.
	ErrAlreadyExists = errors.New("uom category already exists")

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

	// ErrAlreadyDeleted is returned when attempting to modify an already deleted category.
	ErrAlreadyDeleted = errors.New("uom category is already deleted")

	// ErrInUse is returned when attempting to delete a category that is still referenced by UOMs.
	ErrInUse = errors.New("uom category is still in use by one or more UOMs")
)
