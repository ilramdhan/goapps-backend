// Package rmcategory provides domain logic for Raw Material Category management.
package rmcategory

import (
	"regexp"
	"strings"
)

// codePattern validates the code format: starts with uppercase letter,
// followed by uppercase letters, digits, or underscores.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Code represents a validated raw material category code.
type Code struct {
	value string
}

// NewCode creates a new Code value object with validation.
func NewCode(code string) (Code, error) {
	code = strings.TrimSpace(code)

	if code == "" {
		return Code{}, ErrEmptyCode
	}

	if len(code) > 20 {
		return Code{}, ErrCodeTooLong
	}

	// Normalize to uppercase
	code = strings.ToUpper(code)

	if !codePattern.MatchString(code) {
		return Code{}, ErrInvalidCodeFormat
	}

	return Code{value: code}, nil
}

// String returns the string representation of the code.
func (c Code) String() string { return c.value }

// IsEmpty returns true if the code is empty.
func (c Code) IsEmpty() bool { return c.value == "" }

// Equal returns true if the two codes are equal.
func (c Code) Equal(other Code) bool { return c.value == other.value }
