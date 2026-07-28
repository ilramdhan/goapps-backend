// Package lot provides domain logic for PPC lot-master data.
package lot

import "errors"

// Domain errors for lot-master operations.
var (
	// ErrNotFound is returned when a lot master is not found.
	ErrNotFound = errors.New("lot master not found")

	// ErrAlreadyExists is returned when a lot master with the same lot number already exists.
	ErrAlreadyExists = errors.New("lot master already exists")

	// ErrEmptyLotNo is returned when the lot number is empty.
	ErrEmptyLotNo = errors.New("lot number cannot be empty")

	// ErrLotNoTooLong is returned when the lot number exceeds max length.
	ErrLotNoTooLong = errors.New("lot number must be at most 30 characters")

	// ErrEmptyItemCode is returned when the item code is empty.
	ErrEmptyItemCode = errors.New("item code cannot be empty")

	// ErrItemCodeTooLong is returned when the item code exceeds max length.
	ErrItemCodeTooLong = errors.New("item code must be at most 30 characters")

	// ErrEmptyShadeCode is returned when the shade code is empty.
	ErrEmptyShadeCode = errors.New("shade code cannot be empty")

	// ErrShadeCodeTooLong is returned when the shade code exceeds max length.
	ErrShadeCodeTooLong = errors.New("shade code must be at most 20 characters")

	// ErrInvalidWeight is returned when a standard weight is not greater than 0.
	ErrInvalidWeight = errors.New("standard weight must be greater than 0")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrSyncNotConfigured is returned when an Oracle lot sync is requested but
	// no sync usecase was wired (Oracle unconfigured at startup).
	ErrSyncNotConfigured = errors.New("lot sync is not configured")
)
