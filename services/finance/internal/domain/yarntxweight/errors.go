// Package yarntxweight provides domain logic for the global TX Weight master.
package yarntxweight

import "errors"

// Domain errors for TX Weight operations.
var (
	// ErrNotFound is returned when a TX Weight rule is not found.
	ErrNotFound = errors.New("tx weight rule not found")
	// ErrAlreadyExists is returned when a live rule already exists for the product type and grade.
	ErrAlreadyExists = errors.New("tx weight rule already exists for this product type and grade")
	// ErrInvalidProductType is returned when the product type id is not positive.
	ErrInvalidProductType = errors.New("invalid tx weight product type: required")
	// ErrProductTypeNotFound is returned when the referenced product type does not exist.
	ErrProductTypeNotFound = errors.New("tx weight product type not found")
	// ErrInvalidGrade is returned for a grade outside AE/A9/A/B/C.
	ErrInvalidGrade = errors.New("invalid tx weight grade: must be one of AE, A9, A, B, C")
	// ErrInvalidMode is returned for a mode outside LESS_BY/MULTIPLY/FIXED.
	ErrInvalidMode = errors.New("invalid tx weight mode: must be one of LESS_BY, MULTIPLY, FIXED")
	// ErrInvalidValue is returned for a non-finite value.
	ErrInvalidValue = errors.New("invalid tx weight value: must be a finite number")
	// ErrDescriptionTooLong is returned when the description exceeds 200 characters.
	ErrDescriptionTooLong = errors.New("invalid tx weight description: must be at most 200 characters")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted rule.
	ErrAlreadyDeleted = errors.New("tx weight rule is already deleted")
)
