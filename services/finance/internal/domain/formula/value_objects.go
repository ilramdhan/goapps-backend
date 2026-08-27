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
	// TypeMBCostLookup identifies downstream POY-consumer formulas reading cst_mb_cost
	// (migration 000451). Not used by the 7 F_MB_* formulas themselves — those are
	// RM_LOOKUP/CALCULATION.
	TypeMBCostLookup = Type{value: "MB_COST_LOOKUP"}
	// TypeMBXSectionLookup identifies the MB LDR cross-section conversion formula
	// F_MB_LDR_XSECTION (migration 000480). Like RM_LOOKUP/MB_COST_LOOKUP it needs
	// dedicated Go code — a master-data lookup into mst_mb_cross_section_factor —
	// not plain expr-lang evaluation of its stored expression.
	TypeMBXSectionLookup = Type{value: "MB_XSECTION_LOOKUP"}
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
	case "MB_COST_LOOKUP":
		return TypeMBCostLookup, nil
	case "MB_XSECTION_LOOKUP":
		return TypeMBXSectionLookup, nil
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
