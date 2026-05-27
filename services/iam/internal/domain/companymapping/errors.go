// Package companymapping provides domain logic for Company Mapping management.
package companymapping

import "errors"

// Domain-specific errors for company mapping package.
var (
	// ErrInvalidCodeFormat is returned when the code does not match the expected pattern.
	ErrInvalidCodeFormat = errors.New("company mapping code must start with an uppercase letter and contain only uppercase letters, digits and hyphens")
	// ErrComboTaken indicates another mapping already exists with the same
	// (company, division, department, section) combination.
	ErrComboTaken = errors.New("a company mapping with the same hierarchy already exists")
	// ErrAssignedToUser indicates the mapping cannot be deleted because at
	// least one user still references it.
	ErrAssignedToUser = errors.New("company mapping is still assigned to one or more users")
	// ErrInvalidCompanyID indicates a missing/zero company id.
	ErrInvalidCompanyID = errors.New("company id is required")
	// ErrInvalidDivisionID indicates a missing/zero division id.
	ErrInvalidDivisionID = errors.New("division id is required")
	// ErrInvalidDepartmentID indicates a missing/zero department id.
	ErrInvalidDepartmentID = errors.New("department id is required")
)
