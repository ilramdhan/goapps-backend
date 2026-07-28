// Package capacity provides domain logic for PPC product-machine-capacity master data.
package capacity

import "errors"

// Domain errors for product-machine-capacity operations.
var (
	// ErrNotFound is returned when a capacity is not found.
	ErrNotFound = errors.New("product machine capacity not found")

	// ErrAlreadyExists is returned when a capacity for the same product and machine already exists.
	ErrAlreadyExists = errors.New("capacity for product and machine already exists")

	// ErrInvalidProduct is returned when the product system ID is not valid.
	ErrInvalidProduct = errors.New("invalid cpm_product_sys_id")

	// ErrInvalidMachine is returned when the machine ID is not valid.
	ErrInvalidMachine = errors.New("invalid machine_id")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
