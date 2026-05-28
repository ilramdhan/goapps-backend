package upload

import "errors"

// Sentinel errors for the upload domain.
var (
	// ErrNotFound is returned when an upload session does not exist.
	ErrNotFound = errors.New("upload session not found")
	// ErrNotCommittable is returned when a session is not in a state that allows commit.
	ErrNotCommittable = errors.New("upload session is not in a committable state")
	// ErrNotCancellable is returned when a session cannot be cancelled (already committed).
	ErrNotCancellable = errors.New("upload session cannot be cancelled")
	// ErrTooManyRows is returned when the uploaded file exceeds the row limit.
	ErrTooManyRows = errors.New("upload exceeds the maximum allowed rows")
	// ErrInvalidTargetType is returned when the target type is empty.
	ErrInvalidTargetType = errors.New("target type is required")
	// ErrNoDataRows is returned when the file has no data rows.
	ErrNoDataRows = errors.New("upload contains no data rows")
)
