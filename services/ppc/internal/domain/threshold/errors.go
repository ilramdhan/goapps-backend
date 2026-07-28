// Package threshold provides domain logic for PPC overrun-threshold-config master data.
package threshold

import "errors"

// Domain errors for overrun-threshold-config operations.
var (
	// ErrNotFound is returned when an overrun threshold config is not found.
	ErrNotFound = errors.New("overrun threshold config not found")

	// ErrInvalidLevel is returned when the threshold level is not one of the allowed values.
	ErrInvalidLevel = errors.New("invalid threshold level")

	// ErrInvalidUnit is returned when the threshold unit is not PCT or DOFF.
	ErrInvalidUnit = errors.New("invalid threshold unit: must be PCT or DOFF")

	// ErrInvalidThresholds is returned when the block value is less than the warning value.
	ErrInvalidThresholds = errors.New("block value must be greater than or equal to warning value")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
