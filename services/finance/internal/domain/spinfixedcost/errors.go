// Package spinfixedcost provides domain logic for the POY spinning fixed-cost pool master.
package spinfixedcost

import "errors"

// Domain errors for Spin Fixed Cost operations.
//
// NOTE: internal/delivery/grpc/error_response.go maps these to HTTP status codes by
// STRING-MATCHING the message text ("not found" -> 404, "already exists" -> 409,
// "invalid" -> 400). Every message below must keep one of those substrings.
var (
	// ErrNotFound is returned when a spin fixed cost record is not found.
	ErrNotFound = errors.New("spin fixed cost not found")
	// ErrDuplicatePeriod is returned when a live record already exists for the period.
	ErrDuplicatePeriod = errors.New("spin fixed cost for this period already exists")
	// ErrInvalidPeriod is returned when the period is not in YYYYMM format.
	ErrInvalidPeriod = errors.New("invalid period: must be 6 digits in YYYYMM format")
	// ErrNegativeAmount is returned when a monthly cost component is negative.
	ErrNegativeAmount = errors.New("invalid amount: monthly cost components must not be negative")
	// ErrNonPositiveDenier is returned when common_poy_denier is zero or negative.
	ErrNonPositiveDenier = errors.New("invalid common_poy_denier: must be greater than zero (calc engine divisor)")
	// ErrNonPositiveProduction is returned when poy_production is zero or negative.
	ErrNonPositiveProduction = errors.New("invalid poy_production: must be greater than zero (calc engine divisor)")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("invalid created_by: cannot be empty")
	// ErrAlreadyDeleted is returned when attempting to modify an already deleted record.
	ErrAlreadyDeleted = errors.New("invalid operation: spin fixed cost is already deleted")

	// ErrAnchorRowOnly is returned when deleting/deactivating the last live+active row.
	// See CheckAnchorGuard for why this is fatal rather than merely undesirable.
	ErrAnchorRowOnly = errors.New(
		"invalid operation: this is the only active spin fixed cost row; " +
			"removing or deactivating it would silently zero POY fixed cost for every POY product " +
			"instead of raising an error",
	)
	// ErrAnchorRowEarliest is returned when deleting/deactivating the earliest live+active row
	// while later live rows still depend on it as the fallback anchor.
	ErrAnchorRowEarliest = errors.New(
		"invalid operation: this is the earliest active spin fixed cost row and later rows exist; " +
			"removing or deactivating it would leave earlier periods with no pool and silently zero " +
			"POY fixed cost for those periods instead of raising an error",
	)
)
