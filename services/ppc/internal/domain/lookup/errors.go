// Package lookup provides domain logic for the PPC lookup master.
package lookup

import "errors"

// Domain errors for lookup operations.
var (
	// ErrNotFound is returned when a lookup row is not found.
	ErrNotFound = errors.New("lookup not found")

	// ErrAlreadyExists is returned when a lookup with the same category and code already exists.
	ErrAlreadyExists = errors.New("lookup already exists")

	// ErrEmptyCategory is returned when the category is empty.
	ErrEmptyCategory = errors.New("lookup category cannot be empty")

	// ErrCategoryTooLong is returned when the category exceeds max length.
	ErrCategoryTooLong = errors.New("lookup category must be at most 40 characters")

	// ErrEmptyCode is returned when the code is empty.
	ErrEmptyCode = errors.New("lookup code cannot be empty")

	// ErrCodeTooLong is returned when the code exceeds max length.
	ErrCodeTooLong = errors.New("lookup code must be at most 40 characters")

	// ErrEmptyLabel is returned when the label is empty.
	ErrEmptyLabel = errors.New("lookup label cannot be empty")

	// ErrLabelTooLong is returned when the label exceeds max length.
	ErrLabelTooLong = errors.New("lookup label must be at most 120 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
