// Package shift provides domain logic for the PPC shift master.
package shift

import "errors"

// Domain errors for shift operations.
var (
	// ErrNotFound is returned when a shift is not found.
	ErrNotFound = errors.New("shift not found")

	// ErrAlreadyExists is returned when a shift with the same code already exists.
	ErrAlreadyExists = errors.New("shift already exists")

	// ErrInvalidCode is returned when the code is not a single digit.
	ErrInvalidCode = errors.New("invalid shift code: must be a single digit")

	// ErrNameTooLong is returned when the name exceeds max length.
	ErrNameTooLong = errors.New("shift name must be at most 40 characters")

	// ErrInvalidStartTime is returned when the start time is not valid HH:MM.
	ErrInvalidStartTime = errors.New("invalid start time: must be HH:MM (24-hour)")

	// ErrInvalidEndTime is returned when the end time is not valid HH:MM.
	ErrInvalidEndTime = errors.New("invalid end time: must be HH:MM (24-hour)")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
