// Package formula provides domain logic for Formula management.
package formula

import "errors"

// Domain errors for Formula operations.
var (
	// ErrNotFound is returned when a formula is not found.
	ErrNotFound = errors.New("formula not found")

	// ErrAlreadyExists is returned when attempting to create a formula with an existing code.
	ErrAlreadyExists = errors.New("formula already exists")

	// ErrEmptyCode is returned when the formula code is empty.
	ErrEmptyCode = errors.New("formula code cannot be empty")

	// ErrInvalidCodeFormat is returned when the formula code format is invalid.
	ErrInvalidCodeFormat = errors.New("formula code must start with uppercase letter and contain only uppercase letters, numbers, and underscores")

	// ErrCodeTooLong is returned when the formula code exceeds max length.
	ErrCodeTooLong = errors.New("formula code must be at most 50 characters")

	// ErrEmptyName is returned when the formula name is empty.
	ErrEmptyName = errors.New("formula name cannot be empty")

	// ErrNameTooLong is returned when the formula name exceeds max length.
	ErrNameTooLong = errors.New("formula name must be at most 200 characters")

	// ErrInvalidFormulaType is returned when the formula type is invalid.
	ErrInvalidFormulaType = errors.New("invalid formula type: must be CALCULATION, SQL_QUERY, or CONSTANT")

	// ErrEmptyExpression is returned when the expression is empty.
	ErrEmptyExpression = errors.New("expression cannot be empty")

	// ErrExpressionTooLong is returned when the expression exceeds max length.
	ErrExpressionTooLong = errors.New("expression must be at most 5000 characters")

	// ErrEmptyResultParam is returned when the result parameter ID is empty.
	ErrEmptyResultParam = errors.New("result parameter ID is required")

	// ErrResultParamNotFound is returned when the referenced result parameter does not exist.
	ErrResultParamNotFound = errors.New("result parameter not found")

	// ErrInputParamNotFound is returned when a referenced input parameter does not exist.
	ErrInputParamNotFound = errors.New("input parameter not found")

	// ErrResultParamAlreadyUsed is returned when the result parameter is already used by another formula.
	ErrResultParamAlreadyUsed = errors.New("result parameter is already used by another formula")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrAlreadyDeleted is returned when attempting to modify an already deleted formula.
	ErrAlreadyDeleted = errors.New("formula is already deleted")

	// ErrInvalidExpression is returned when expression references unknown parameter codes.
	ErrInvalidExpression = errors.New("expression contains invalid parameter references")

	// ErrDescriptionTooLong is returned when the description exceeds max length.
	ErrDescriptionTooLong = errors.New("description must be at most 1000 characters")

	// ErrDuplicateInputParam is returned when an input parameter is referenced more than once.
	ErrDuplicateInputParam = errors.New("duplicate input parameter")

	// ErrCircularReference is returned when result param is also an input param.
	ErrCircularReference = errors.New("result parameter cannot be used as an input parameter")
)
