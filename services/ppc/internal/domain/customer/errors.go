// Package customer provides domain logic for the PPC customer master.
package customer

import "errors"

// Domain errors for customer operations.
var (
	// ErrNotFound is returned when a customer is not found.
	ErrNotFound = errors.New("customer not found")

	// ErrAlreadyExists is returned when a customer with the same code already exists.
	ErrAlreadyExists = errors.New("customer already exists")

	// ErrEmptyCode is returned when the customer code is empty.
	ErrEmptyCode = errors.New("customer code cannot be empty")

	// ErrCodeTooLong is returned when the customer code exceeds max length.
	ErrCodeTooLong = errors.New("customer code must be at most 30 characters")

	// ErrEmptyName is returned when the customer name is empty.
	ErrEmptyName = errors.New("customer name cannot be empty")

	// ErrNameTooLong is returned when the customer name exceeds max length.
	ErrNameTooLong = errors.New("customer name must be at most 240 characters")

	// ErrShortNameTooLong is returned when the short name exceeds max length.
	ErrShortNameTooLong = errors.New("customer short name must be at most 60 characters")

	// ErrTaxNoTooLong is returned when the tax registration number exceeds max length.
	ErrTaxNoTooLong = errors.New("customer tax number must be at most 60 characters")

	// ErrParentCodeTooLong is returned when the parent customer code exceeds max length.
	ErrParentCodeTooLong = errors.New("customer parent code must be at most 30 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrSyncNotConfigured is returned when an Oracle sync is requested but no
	// Oracle source is wired (host unset or unreachable at startup).
	ErrSyncNotConfigured = errors.New("customer sync is not configured")
)
