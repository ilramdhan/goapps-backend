// Package parameter provides domain logic for Parameter management.
package parameter

import "errors"

// Domain errors for Parameter operations.
var (
	// ErrNotFound is returned when a parameter is not found.
	ErrNotFound = errors.New("parameter not found")

	// ErrAlreadyExists is returned when attempting to create a parameter with an existing code.
	ErrAlreadyExists = errors.New("parameter already exists")

	// ErrEmptyCode is returned when the parameter code is empty.
	ErrEmptyCode = errors.New("parameter code cannot be empty")

	// ErrInvalidCodeFormat is returned when the parameter code format is invalid.
	ErrInvalidCodeFormat = errors.New("parameter code must start with uppercase letter and contain only uppercase letters, numbers, and underscores")

	// ErrCodeTooLong is returned when the parameter code exceeds max length.
	ErrCodeTooLong = errors.New("parameter code must be at most 20 characters")

	// ErrInvalidDataType is returned when the data type is invalid.
	ErrInvalidDataType = errors.New("invalid data type: must be NUMBER, TEXT, or BOOLEAN")

	// ErrInvalidParamCategory is returned when the parameter category is invalid.
	ErrInvalidParamCategory = errors.New("invalid parameter category: must be INPUT, RATE, CALCULATED, or MASTER_LOOKUP")

	// ErrEmptyName is returned when the parameter name is empty.
	ErrEmptyName = errors.New("parameter name cannot be empty")

	// ErrNameTooLong is returned when the parameter name exceeds max length.
	ErrNameTooLong = errors.New("parameter name must be at most 200 characters")

	// ErrShortNameTooLong is returned when the short name exceeds max length.
	ErrShortNameTooLong = errors.New("parameter short name must be at most 50 characters")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrAlreadyDeleted is returned when attempting to modify an already deleted parameter.
	ErrAlreadyDeleted = errors.New("parameter is already deleted")

	// ErrInvalidMinMax is returned when min_value is greater than max_value.
	ErrInvalidMinMax = errors.New("min_value cannot be greater than max_value")

	// ErrUOMNotFound is returned when a referenced UOM code does not exist.
	ErrUOMNotFound = errors.New("referenced UOM not found")

	// ErrOwnerDepartmentTooLong is returned when owner_department exceeds 30 chars.
	ErrOwnerDepartmentTooLong = errors.New("owner_department must be at most 30 characters")

	// ErrLookupMasterCodeTooLong is returned when lookup_master_code exceeds 30 chars.
	ErrLookupMasterCodeTooLong = errors.New("lookup_master_code must be at most 30 characters")

	// ErrLookupFillGroupCodeTooLong is returned when lookup_fill_group_code exceeds 20 chars.
	ErrLookupFillGroupCodeTooLong = errors.New("lookup_fill_group_code must be at most 20 characters")

	// ErrLookupSourceColumnTooLong is returned when lookup_source_column exceeds 50 chars.
	ErrLookupSourceColumnTooLong = errors.New("lookup_source_column must be at most 50 characters")

	// ErrDisplayGroupTooLong is returned when display_group exceeds 50 chars.
	ErrDisplayGroupTooLong = errors.New("display_group must be at most 50 characters")

	// ErrInvalidDisplayOrder is returned when display_order is negative.
	ErrInvalidDisplayOrder = errors.New("display_order must be non-negative")

	// ErrNotesTooLong is returned when notes exceeds 500 chars.
	ErrNotesTooLong = errors.New("notes must be at most 500 characters")
)
