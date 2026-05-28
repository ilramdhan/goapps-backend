package group

import "errors"

// Sentinel errors for the DashboardGroup aggregate.
var (
	// ErrNotFound is returned when no group matches a lookup.
	ErrNotFound = errors.New("dashboard group not found")
	// ErrAlreadyExists is returned on group_code uniqueness violation.
	ErrAlreadyExists = errors.New("dashboard group code already exists")
	// ErrInvalidCode is returned on group_code validation failure.
	ErrInvalidCode = errors.New("invalid dashboard group code")
	// ErrInvalidName is returned when group_name is empty or too long.
	ErrInvalidName = errors.New("invalid dashboard group name")
	// ErrInUse is returned by SoftDelete when one or more active dashboards reference the group.
	ErrInUse = errors.New("dashboard group is in use")
)
