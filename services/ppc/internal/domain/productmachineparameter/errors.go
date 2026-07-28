// Package productmachineparameter provides domain logic for the PPC
// product-machine-parameter master (per product+machine typed parameter values).
package productmachineparameter

import "errors"

// Domain errors for product-machine-parameter operations.
var (
	// ErrNotFound is returned when a product-machine-parameter is not found.
	ErrNotFound = errors.New("product machine parameter not found")

	// ErrAlreadyExists is returned when a value for the same product, machine
	// and parameter already exists.
	ErrAlreadyExists = errors.New("product machine parameter already exists")

	// ErrInvalidProduct is returned when the product system ID is not valid.
	ErrInvalidProduct = errors.New("invalid cpm_product_sys_id")

	// ErrInvalidMachine is returned when the machine ID is not valid.
	ErrInvalidMachine = errors.New("invalid machine_id")

	// ErrInvalidParam is returned when the parameter ID is not a valid UUID.
	ErrInvalidParam = errors.New("invalid param_id: must be a valid UUID")
)
