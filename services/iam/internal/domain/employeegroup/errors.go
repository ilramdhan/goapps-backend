// Package employeegroup provides domain logic for Employee Group management.
package employeegroup

import "errors"

// Domain-specific errors for employee group package.
var (
	// ErrInvalidCodeFormat is returned when the code does not match the expected pattern.
	ErrInvalidCodeFormat = errors.New("employee group code must start with an uppercase letter and contain only uppercase letters and digits")
)
