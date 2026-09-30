// Package yarntxweightgroup provides domain logic for TX Weight groups: one
// config (a set of AE/A9/A/B/C grade rules) shared by many product types.
package yarntxweightgroup

import (
	"errors"
	"fmt"
)

// Domain errors for TX Weight group operations. Messages keep the "not found",
// "already exists", "already assigned" and "invalid" phrases the gRPC layer
// maps to 404 / 409 / 400 (see domainErrorToBaseResponse).
var (
	// ErrNotFound is returned when a TX Weight group is not found.
	ErrNotFound = errors.New("tx weight group not found")
	// ErrCodeAlreadyExists is returned when a live group already uses the code.
	ErrCodeAlreadyExists = errors.New("tx weight group code already exists")
	// ErrProductTypeAlreadyMapped is the sentinel every ProductTypeConflictError
	// unwraps to; it is also returned bare when a concurrent write wins the
	// UNIQUE(product_type_id) race.
	ErrProductTypeAlreadyMapped = errors.New("product type is already assigned to another tx weight group")
	// ErrProductTypeNotFound is returned when a referenced product type does not exist.
	ErrProductTypeNotFound = errors.New("tx weight group product type not found")
	// ErrInvalidCode is returned for an empty, too long or malformed code.
	ErrInvalidCode = errors.New("invalid tx weight group code: must be 1-30 chars of A-Z, 0-9, _ starting with a letter")
	// ErrInvalidName is returned for an empty or too long name.
	ErrInvalidName = errors.New("invalid tx weight group name: must be 1-100 characters")
	// ErrDescriptionTooLong is returned when a description exceeds 200 characters.
	ErrDescriptionTooLong = errors.New("invalid tx weight group description: must be at most 200 characters")
	// ErrNoProductTypes is returned when no product type is given.
	ErrNoProductTypes = errors.New("invalid tx weight group product types: at least one is required")
	// ErrInvalidProductType is returned for a non-positive product type id.
	ErrInvalidProductType = errors.New("invalid tx weight group product type: id must be positive")
	// ErrDuplicateProductType is returned when a product type id is repeated.
	ErrDuplicateProductType = errors.New("invalid tx weight group product types: each product type may appear at most once")
	// ErrInvalidRuleCount is returned when rules are missing or more than five.
	ErrInvalidRuleCount = errors.New("invalid tx weight group rules: 1 to 5 rules are required")
	// ErrDuplicateGrade is returned when a grade appears in more than one rule.
	ErrDuplicateGrade = errors.New("invalid tx weight group rules: each grade may appear at most once")
	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
	// ErrAlreadyDeleted is returned when modifying an already deleted group.
	ErrAlreadyDeleted = errors.New("tx weight group is already deleted")
)

// ProductTypeConflict names a product type already owned by another live group.
type ProductTypeConflict struct {
	ProductTypeID   int32
	ProductTypeCode string
	GroupCode       string
}

// ProductTypeConflictError reports every requested product type that another
// live group already owns. It unwraps to ErrProductTypeAlreadyMapped.
type ProductTypeConflictError struct {
	Conflicts []ProductTypeConflict
}

// Error names each type code and the group code that owns it.
func (e *ProductTypeConflictError) Error() string {
	msg := ""
	for i, c := range e.Conflicts {
		if i > 0 {
			msg += "; "
		}
		msg += fmt.Sprintf("product type %s is already assigned to tx weight group %s", c.ProductTypeCode, c.GroupCode)
	}
	return msg
}

// Unwrap lets errors.Is(err, ErrProductTypeAlreadyMapped) match.
func (e *ProductTypeConflictError) Unwrap() error { return ErrProductTypeAlreadyMapped }
