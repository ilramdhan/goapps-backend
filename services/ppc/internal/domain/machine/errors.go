// Package machine provides domain logic for PPC machine master data.
package machine

import "errors"

// Domain errors for machine operations.
var (
	// ErrNotFound is returned when a machine is not found.
	ErrNotFound = errors.New("machine not found")

	// ErrInvalidArea is returned when the area code is not one of TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")

	// ErrEmptyUpdatedBy is returned when updated_by is empty.
	ErrEmptyUpdatedBy = errors.New("updated_by cannot be empty")
)
