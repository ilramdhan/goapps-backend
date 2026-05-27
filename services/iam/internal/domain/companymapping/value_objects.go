// Package companymapping provides domain logic for Company Mapping management.
package companymapping

import (
	"regexp"
	"strings"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

const (
	maxCodeLen = 50
	maxNameLen = 200
)

// codePattern validates code format: starts with uppercase letter,
// followed by uppercase letters, digits, or hyphens.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9-]*$`)

// Code represents a validated company mapping code.
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

// Name represents a validated company mapping name.
type Name struct {
	value string
}

// NewName creates a new validated Name.
func NewName(name string) (Name, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Name{}, shared.ErrEmptyName
	}
	if len(name) > maxNameLen {
		return Name{}, shared.ErrNameTooLong
	}
	return Name{value: name}, nil
}

// String returns the string representation of the name.
func (n Name) String() string { return n.value }

// IsEmpty returns true if the name is empty.
func (n Name) IsEmpty() bool { return n.value == "" }
