// Package wastecategory provides domain logic for PPC waste-category master data.
package wastecategory

import "errors"

// Domain errors for waste-category operations.
var (
	// ErrNotFound is returned when a waste category is not found.
	ErrNotFound = errors.New("waste category not found")

	// ErrAlreadyExists is returned when a waste category with the same area, type, and code already exists.
	ErrAlreadyExists = errors.New("waste category already exists")

	// ErrInvalidArea is returned when the area code is not one of TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")

	// ErrInvalidType is returned when the type is not WASTE or DOWNGRADE.
	ErrInvalidType = errors.New("invalid type: must be WASTE or DOWNGRADE")

	// ErrEmptyCode is returned when the category code is empty.
	ErrEmptyCode = errors.New("waste category code cannot be empty")

	// ErrCodeTooLong is returned when the category code exceeds max length.
	ErrCodeTooLong = errors.New("waste category code must be at most 30 characters")

	// ErrEmptyName is returned when the category name is empty.
	ErrEmptyName = errors.New("waste category name cannot be empty")

	// ErrNameTooLong is returned when the category name exceeds max length.
	ErrNameTooLong = errors.New("waste category name must be at most 100 characters")

	// ErrGradeTargetRequired is returned when a DOWNGRADE category has no grade target.
	ErrGradeTargetRequired = errors.New("grade target is required for DOWNGRADE type")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
