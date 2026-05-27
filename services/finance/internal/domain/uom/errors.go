// Package uom provides domain logic for Unit of Measure management.
package uom

import "errors"

// Domain errors for UOM operations.
var (
	// ErrNotFound is returned when a UOM is not found.
	ErrNotFound = errors.New("uom not found")

	// ErrAlreadyExists is returned when attempting to create a UOM with an existing code.
	ErrAlreadyExists = errors.New("uom already exists")

	// ErrInvalidCode is returned when the UOM code format is invalid.
	ErrInvalidCode = errors.New("invalid uom code: must be uppercase alphanumeric with underscores, 1-20 chars")

	// ErrEmptyCode is returned when the UOM code is empty.
	ErrEmptyCode = errors.New("uom code cannot be empty")

	// ErrInvalidCodeFormat is returned when the UOM code format is invalid.
	ErrInvalidCodeFormat = errors.New("uom code must start with uppercase letter and contain only uppercase letters, numbers, and underscores")

	// ErrCodeTooLong is returned when the UOM code exceeds max length.
	ErrCodeTooLong = errors.New("uom code must be at most 20 characters")

	// ErrInvalidCategory is returned when the UOM category ID is invalid.
	ErrInvalidCategory = errors.New("invalid uom category: category ID is required")

	// ErrEmptyName is returned when the UOM name is empty.
	ErrEmptyName = errors.New("uom name cannot be empty")

	// ErrNameTooLong is returned when the UOM name exceeds max length.
	ErrNameTooLong = errors.New("uom name must be at most 100 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrEmptyUpdatedBy is returned when updated_by is empty.
	ErrEmptyUpdatedBy = errors.New("updated_by cannot be empty")

	// ErrAlreadyDeleted is returned when attempting to delete an already deleted UOM.
	ErrAlreadyDeleted = errors.New("uom is already deleted")
)
