// Package shade provides domain logic for the shade master (cost_erp_shade).
package shade

import "errors"

// Domain errors for shade operations.
var (
	// ErrNotFound is returned when a shade is not found.
	ErrNotFound = errors.New("shade not found")

	// ErrAlreadyExists is returned when a shade with the same code already exists.
	ErrAlreadyExists = errors.New("shade already exists")

	// ErrEmptyCode is returned when the shade code is empty.
	ErrEmptyCode = errors.New("shade code cannot be empty")

	// ErrCodeTooLong is returned when the shade code exceeds max length.
	ErrCodeTooLong = errors.New("shade code must be at most 60 characters")

	// ErrEmptyName is returned when the shade name is empty.
	ErrEmptyName = errors.New("shade name cannot be empty")

	// ErrNameTooLong is returned when the shade name exceeds max length.
	ErrNameTooLong = errors.New("shade name must be at most 300 characters")

	// ErrShortNameTooLong is returned when the short name exceeds max length.
	ErrShortNameTooLong = errors.New("shade short name must be at most 60 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrSyncNotConfigured is returned when an Oracle sync is requested but no
	// Oracle source is wired (host unset or unreachable at startup).
	ErrSyncNotConfigured = errors.New("shade sync is not configured")
)
