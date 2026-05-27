// Package rmgroup provides domain logic for raw-material grouping and landed-cost configuration.
package rmgroup

import "errors"

// Domain errors for RM group operations.
var (
	// ErrNotFound is returned when an RM group head is not found.
	ErrNotFound = errors.New("rm group not found")

	// ErrCodeAlreadyExists is returned when a group is created with a code that is already in use.
	ErrCodeAlreadyExists = errors.New("rm group code already exists")

	// ErrAlreadyDeleted is returned when attempting to modify a soft-deleted group.
	ErrAlreadyDeleted = errors.New("rm group is already deleted")

	// ErrEmptyCode is returned when the group code is empty.
	ErrEmptyCode = errors.New("rm group code cannot be empty")

	// ErrInvalidCodeFormat is returned when the group code does not match the allowed pattern.
	ErrInvalidCodeFormat = errors.New("rm group code must start with an uppercase letter or digit and may only contain uppercase letters, digits, spaces, and hyphens")

	// ErrCodeTooLong is returned when the group code exceeds the max length.
	ErrCodeTooLong = errors.New("rm group code must be at most 30 characters")

	// ErrEmptyName is returned when the group name is empty.
	ErrEmptyName = errors.New("rm group name cannot be empty")

	// ErrNameTooLong is returned when the group name exceeds the max length.
	ErrNameTooLong = errors.New("rm group name must be at most 200 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrEmptyUpdatedBy is returned when updated_by is empty.
	ErrEmptyUpdatedBy = errors.New("updated_by cannot be empty")

	// ErrInvalidFlag is returned when a stage flag is not one of the allowed values.
	ErrInvalidFlag = errors.New("invalid stage flag — must be one of CONS, STORES, DEPT, PO_1, PO_2, PO_3, INIT")

	// ErrInitValueRequired is returned when a flag is set to INIT without a corresponding init_val.
	ErrInitValueRequired = errors.New("init value must be provided when the flag is INIT")

	// ErrNegativeCostPercentage is returned when cost_percentage is negative.
	ErrNegativeCostPercentage = errors.New("cost percentage must be non-negative")

	// ErrNegativeCostPerKg is returned when cost_per_kg is negative.
	ErrNegativeCostPerKg = errors.New("cost per kg must be non-negative")

	// ErrNegativeInitValue is returned when an init value is negative.
	ErrNegativeInitValue = errors.New("init value must be non-negative")

	// ErrEmptyItemCode is returned when an item code is empty.
	ErrEmptyItemCode = errors.New("item code cannot be empty")

	// ErrItemCodeTooLong is returned when an item code exceeds the max length.
	ErrItemCodeTooLong = errors.New("item code must be at most 20 characters")

	// ErrItemAlreadyInOtherGroup is returned when adding an item that is already assigned
	// to another active group. Callers should consult the repository to identify the owning group.
	ErrItemAlreadyInOtherGroup = errors.New("item is already assigned to another active group")

	// ErrDetailNotFound is returned when a group detail (item membership) is not found.
	ErrDetailNotFound = errors.New("rm group detail not found")

	// ErrNegativeMarketPercentage is returned when market_percentage is negative.
	ErrNegativeMarketPercentage = errors.New("market percentage must be non-negative")

	// ErrNegativeMarketValue is returned when market_value_rp is negative.
	ErrNegativeMarketValue = errors.New("market value must be non-negative")

	// ErrGroupHasCostData is returned when attempting to delete a group head
	// that has already produced cost calculation rows. Deleting would orphan
	// the historical cost audit trail, so the operation is blocked.
	ErrGroupHasCostData = errors.New("rm group cannot be deleted: cost data has already been generated for this group")
)
