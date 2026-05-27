// Package employeegroup provides domain logic for Employee Group management.
package employeegroup

import (
	"regexp"
	"strings"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

const maxCodeLen = 20

// codePattern validates code format: starts with uppercase letter,
// followed by uppercase letters or digits (no hyphens, no underscores).
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// Code represents a validated employee group code.
type Code struct {
	value string
}

// NewCode creates a new Code value object with validation.
func NewCode(code string) (Code, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Code{}, shared.ErrEmptyCode
	}
	if len(code) > maxCodeLen {
		return Code{}, shared.ErrCodeTooLong
	}
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
