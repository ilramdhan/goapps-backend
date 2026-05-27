// Package uom provides domain logic for Unit of Measure management.
package uom

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// =============================================================================
// Code Value Object
// =============================================================================

// Code represents a validated UOM code value object.
type Code struct {
	value string
}

// codePattern defines the valid format for UOM codes.
// Must start with uppercase letter, followed by uppercase letters, digits, or underscores.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// NewCode creates a new validated Code value object.
func NewCode(code string) (Code, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Code{}, ErrEmptyCode
	}
	if len(code) > 20 {
		return Code{}, ErrCodeTooLong
	}
	if !codePattern.MatchString(code) {
		return Code{}, ErrInvalidCodeFormat
	}
	return Code{value: code}, nil
}

// String returns the string representation of the code.
func (c Code) String() string {
	return c.value
}

// IsEmpty returns true if the code is empty.
func (c Code) IsEmpty() bool {
	return c.value == ""
}

// Equals checks if two codes are equal.
func (c Code) Equals(other Code) bool {
	return c.value == other.value
}

// =============================================================================
// CategoryInfo holds denormalized category data from the FK relationship.
// =============================================================================

// CategoryInfo holds the UOM category reference data.
type CategoryInfo struct {
	id   uuid.UUID
	code string
	name string
}

// NewCategoryInfo creates a new CategoryInfo.
func NewCategoryInfo(id uuid.UUID, code, name string) CategoryInfo {
	return CategoryInfo{id: id, code: code, name: name}
}

// ID returns the category UUID.
func (c CategoryInfo) ID() uuid.UUID { return c.id }

// Code returns the category code.
func (c CategoryInfo) Code() string { return c.code }

// Name returns the category name.
func (c CategoryInfo) Name() string { return c.name }
