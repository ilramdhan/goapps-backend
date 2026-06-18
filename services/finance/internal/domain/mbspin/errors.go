// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import "errors"

// Domain errors for MB Spin operations.
var (
	// ErrNotFound is returned when an MB spin record is not found.
	ErrNotFound = errors.New("mb spin not found")
	// ErrAlreadyExists is returned when the oracle_sys_id already exists.
	ErrAlreadyExists = errors.New("mb spin oracle_sys_id already exists")
	// ErrInvalidHeadID is returned when headID is nil UUID.
	ErrInvalidHeadID = errors.New("mb spin head_id cannot be nil")
	// ErrEmptyMgtName is returned when mgt_name is empty.
	ErrEmptyMgtName = errors.New("mb spin mgt_name cannot be empty")
	// ErrMgtNameTooLong is returned when mgt_name exceeds 100 characters.
	ErrMgtNameTooLong = errors.New("mb spin mgt_name must be at most 100 characters")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted spin.
	ErrAlreadyDeleted = errors.New("mb spin is already deleted")
	// ErrHeadNotFound is returned when the referenced MB head does not exist.
	ErrHeadNotFound = errors.New("mb head not found")
)
