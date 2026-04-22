// Package rmcost provides the landed-cost calculation engine and persistence contract
// for the RM cost aggregates produced from grouped raw-material consumption data.
package rmcost

import "errors"

// Domain errors for RM cost operations.
var (
	// ErrNotFound is returned when an RM cost row is not found.
	ErrNotFound = errors.New("rm cost not found")

	// ErrInvalidPeriod is returned when the period string is not a valid YYYYMM value.
	ErrInvalidPeriod = errors.New("period must be a 6-character YYYYMM string")

	// ErrEmptyRMCode is returned when rm_code is empty.
	ErrEmptyRMCode = errors.New("rm_code cannot be empty")

	// ErrInvalidRMType is returned when rm_type is not one of the recognized values.
	ErrInvalidRMType = errors.New("rm_type must be GROUP or ITEM")

	// ErrInvalidStage is returned when a stage value is not one of the recognized values.
	ErrInvalidStage = errors.New("invalid stage — must be one of CONS, STORES, DEPT, PO_1, PO_2, PO_3, INIT")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrEmptyCalculatedBy is returned when calculated_by is empty.
	ErrEmptyCalculatedBy = errors.New("calculated_by cannot be empty")

	// ErrNegativeCost is returned when a computed or supplied cost is negative.
	ErrNegativeCost = errors.New("cost must be non-negative")
)
