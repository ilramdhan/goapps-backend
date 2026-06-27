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
	TypeCalculation   = Type{value: "CALCULATION"}
	TypeSQLQuery      = Type{value: "SQL_QUERY"}
	TypeConstant      = Type{value: "CONSTANT"}
	TypeLookup        = Type{value: "LOOKUP"}
	TypeRMLookup      = Type{value: "RM_LOOKUP"}
	TypeConditional   = Type{value: "CONDITIONAL"}
	TypeFromMarketing = Type{value: "FROM_MARKETING"}
	TypeIntermingling = Type{value: "INTERMINGLING"}
	TypeSnapshot      = Type{value: "SNAPSHOT"}
	TypePending       = Type{value: "PENDING"}
	TypeInitialValue  = Type{value: "INITIAL_VALUE"}
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
	case "LOOKUP":
		return TypeLookup, nil
	case "RM_LOOKUP":
		return TypeRMLookup, nil
	case "CONDITIONAL":
		return TypeConditional, nil
	case "FROM_MARKETING":
		return TypeFromMarketing, nil
	case "INTERMINGLING":
		return TypeIntermingling, nil
	case "SNAPSHOT":
		return TypeSnapshot, nil
	case "PENDING":
		return TypePending, nil
	case "INITIAL_VALUE":
		return TypeInitialValue, nil
	default:
		return Type{}, ErrInvalidFormulaType
	}
}

// String returns the formula type string.
func (f Type) String() string { return f.value }

// IsValid returns true if the formula type is valid.
func (f Type) IsValid() bool {
	_, err := NewType(f.value)
	return err == nil
}
