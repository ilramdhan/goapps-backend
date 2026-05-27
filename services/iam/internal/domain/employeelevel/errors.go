// Package employeelevel provides domain logic for Employee Level management.
package employeelevel

import "errors"

// Domain-specific errors for employee level package.
var (
	// ErrInvalidCodeFormat is returned when the code does not match the expected pattern.
	ErrInvalidCodeFormat = errors.New("employee level code must start with an uppercase letter and contain only uppercase letters, digits, and hyphens")
	// ErrInvalidGrade is returned when the grade is outside the allowed range.
	ErrInvalidGrade = errors.New("employee level grade must be between 0 and 99")
	// ErrInvalidSequence is returned when the sequence is outside the allowed range.
	ErrInvalidSequence = errors.New("employee level sequence must be between 0 and 999")
	// ErrInvalidType is returned when the type is unspecified or unknown.
	ErrInvalidType = errors.New("employee level type is invalid or unspecified")
	// ErrInvalidWorkflow is returned when the workflow state is unspecified or unknown.
	ErrInvalidWorkflow = errors.New("employee level workflow state is invalid or unspecified")
)
