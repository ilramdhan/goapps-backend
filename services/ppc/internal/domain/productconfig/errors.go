// Package productconfig provides domain logic for PPC product-config master data.
package productconfig

import "errors"

// Domain errors for product-config operations.
var (
	// ErrNotFound is returned when a product config is not found.
	ErrNotFound = errors.New("product ppc config not found")

	// ErrAlreadyExists is returned when a config for the same product already exists.
	ErrAlreadyExists = errors.New("product ppc config already exists")

	// ErrInvalidProduct is returned when cpm_product_sys_id is missing or invalid.
	ErrInvalidProduct = errors.New("invalid cpm_product_sys_id: required")

	// ErrNegativePercentage is returned when a yield/buffer/ax percentage is negative.
	ErrNegativePercentage = errors.New("invalid percentage: must be greater than or equal to 0")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")
)
