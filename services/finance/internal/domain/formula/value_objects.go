// Package formula provides domain logic for Formula management.
package formula

import (
	"regexp"
	"strings"
)

// =============================================================================
// Code Value Object
// =============================================================================

var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Code represents a formula code value object.
type Code struct {
	value string
}

// NewCode creates a new validated Code.
func NewCode(s string) (Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Code{}, ErrEmptyCode
	}
	if len(s) > 50 {
		return Code{}, ErrCodeTooLong
	}
	if !codePattern.MatchString(s) {
		return Code{}, ErrInvalidCodeFormat
	}
	return Code{value: s}, nil
}

// String returns the code string.
func (c Code) String() string { return c.value }

// =============================================================================
// Type Value Object
// =============================================================================

// Type represents the type of a formula.
type Type struct {
	value string
}

// Valid formula types.
var (
	TypeCalculation = Type{value: "CALCULATION"}
	TypeSQLQuery    = Type{value: "SQL_QUERY"}
	TypeConstant    = Type{value: "CONSTANT"}
)

// NewType creates a Type from a string.
func NewType(s string) (Type, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	switch s {
	case "CALCULATION":
		return TypeCalculation, nil
	case "SQL_QUERY":
		return TypeSQLQuery, nil
	case "CONSTANT":
		return TypeConstant, nil
	default:
		return Type{}, ErrInvalidFormulaType
	}
}

// String returns the formula type string.
func (f Type) String() string { return f.value }

// IsValid returns true if the formula type is valid.
func (f Type) IsValid() bool {
	return f.value == "CALCULATION" || f.value == "SQL_QUERY" || f.value == "CONSTANT"
}
