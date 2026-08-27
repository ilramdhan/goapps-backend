package mbcrosssection

import "errors"

// Domain errors for MB cross-section operations.
var (
	// ErrCodeRequired is returned when code is empty.
	ErrCodeRequired = errors.New("mbcrosssection: code is required")
	// ErrCodeTooLong is returned when code exceeds the column width.
	ErrCodeTooLong = errors.New("mbcrosssection: code exceeds 10 characters")
	// ErrCreatedByRequired is returned when created_by is empty.
	ErrCreatedByRequired = errors.New("mbcrosssection: created_by is required")
	// ErrUpdatedByRequired is returned when updated_by is empty.
	ErrUpdatedByRequired = errors.New("mbcrosssection: updated_by is required")
	// ErrDeleted is returned when mutating a soft-deleted row.
	ErrDeleted = errors.New("mbcrosssection: record is deleted")
	// ErrAlreadyExists is returned when a cross-section code already exists.
	ErrAlreadyExists = errors.New("mbcrosssection: code already exists")
	// ErrNotFound is returned when a cross-section row is not found.
	ErrNotFound = errors.New("mbcrosssection: not found")
)
