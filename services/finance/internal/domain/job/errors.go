// Package job provides domain logic for general-purpose job execution tracking.
package job

import "errors"

// Domain errors for job operations.
var (
	// ErrNotFound is returned when a job execution is not found.
	ErrNotFound = errors.New("job not found")

	// ErrDuplicateActiveJob is returned when an active job already exists for the same type and period.
	ErrDuplicateActiveJob = errors.New("an active job already exists for this type and period")

	// ErrAlreadyCancelled is returned when attempting to cancel a job that is already cancelled.
	ErrAlreadyCancelled = errors.New("job is already cancelled")

	// ErrAlreadyCompleted is returned when attempting to modify a completed job.
	ErrAlreadyCompleted = errors.New("job is already completed")

	// ErrNotCancellable is returned when a job is in a state that cannot be cancelled.
	ErrNotCancellable = errors.New("job can only be cancelled when queued or processing")

	// ErrInvalidStatus is returned when an invalid status transition is attempted.
	ErrInvalidStatus = errors.New("invalid job status transition")

	// ErrEmptyJobType is returned when the job type is empty.
	ErrEmptyJobType = errors.New("job type cannot be empty")

	// ErrEmptyCreatedBy is returned when the created_by field is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrInvalidPriority is returned when the priority is out of range.
	ErrInvalidPriority = errors.New("priority must be between 1 and 10")
)
