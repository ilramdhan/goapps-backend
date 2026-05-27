// Package employeelevel provides domain logic for Employee Level management.
package employeelevel

import (
	"regexp"
	"strings"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

const (
	maxCodeLen = 20
	maxGrade   = 99
	maxSeq     = 999

	// unspecifiedLabel is the canonical string for unspecified enum values.
	unspecifiedLabel = "UNSPECIFIED"
)

// codePattern validates code format: starts with uppercase letter,
// followed by uppercase letters, digits, or hyphens.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9-]*$`)

// Code represents a validated employee level code.
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

// =============================================================================
// Type (functional category)
// =============================================================================

// Type represents the functional category of an employee level.
type Type int32

// Type enum values. Match proto EmployeeLevelType numeric values.
const (
	// TypeUnspecified is the default / unset value.
	TypeUnspecified Type = 0
	// TypeExecutive is the executive level (Director, GM, Assistant GM).
	TypeExecutive Type = 1
	// TypeNonExecutive is non-executive (Supervisor, Senior Staff).
	TypeNonExecutive Type = 2
	// TypeOperator is the operator level (technicians, skilled operators).
	TypeOperator Type = 3
	// TypeOther is other / uncategorized.
	TypeOther Type = 4
)

// IsValid reports whether the type is a known, non-unspecified value.
func (t Type) IsValid() bool {
	switch t {
	case TypeExecutive, TypeNonExecutive, TypeOperator, TypeOther:
		return true
	case TypeUnspecified:
		return false
	default:
		return false
	}
}

// String returns the canonical string form of the type.
func (t Type) String() string {
	switch t {
	case TypeExecutive:
		return "EXECUTIVE"
	case TypeNonExecutive:
		return "NON_EXECUTIVE"
	case TypeOperator:
		return "OPERATOR"
	case TypeOther:
		return "OTHER"
	case TypeUnspecified:
		return unspecifiedLabel
	default:
		return unspecifiedLabel
	}
}

// ParseType converts a string to a Type. Returns TypeUnspecified for unknown input.
func ParseType(s string) Type {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "EXECUTIVE":
		return TypeExecutive
	case "NON_EXECUTIVE", "NON-EXECUTIVE", "NONEXECUTIVE":
		return TypeNonExecutive
	case "OPERATOR":
		return TypeOperator
	case "OTHER":
		return TypeOther
	default:
		return TypeUnspecified
	}
}

// =============================================================================
// Workflow (lifecycle state)
// =============================================================================

// Workflow represents the lifecycle state of an employee level.
type Workflow int32

// Workflow enum values. Match proto EmployeeLevelWorkflow numeric values.
const (
	// WorkflowUnspecified is the default / unset value.
	WorkflowUnspecified Workflow = 0
	// WorkflowDraft means the record is not yet published.
	WorkflowDraft Workflow = 1
	// WorkflowReleased means the record is active and in use.
	WorkflowReleased Workflow = 2
	// WorkflowSuperUser is reserved for top-level system access.
	WorkflowSuperUser Workflow = 3
	// WorkflowSubmitted means the record has been submitted for approval.
	WorkflowSubmitted Workflow = 4
	// WorkflowApproved means the record has been approved, pending release.
	WorkflowApproved Workflow = 5
)

// IsValid reports whether the workflow is a known, non-unspecified value.
func (w Workflow) IsValid() bool {
	switch w {
	case WorkflowDraft, WorkflowReleased, WorkflowSuperUser, WorkflowSubmitted, WorkflowApproved:
		return true
	case WorkflowUnspecified:
		return false
	default:
		return false
	}
}

// String returns the canonical string form of the workflow.
func (w Workflow) String() string {
	switch w {
	case WorkflowDraft:
		return "DRAFT"
	case WorkflowReleased:
		return "RELEASED"
	case WorkflowSuperUser:
		return "SUPER_USER"
	case WorkflowSubmitted:
		return "SUBMITTED"
	case WorkflowApproved:
		return "APPROVED"
	case WorkflowUnspecified:
		return unspecifiedLabel
	default:
		return unspecifiedLabel
	}
}

// ParseWorkflow converts a string to a Workflow. Returns WorkflowUnspecified for unknown input.
func ParseWorkflow(s string) Workflow {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DRAFT":
		return WorkflowDraft
	case "RELEASED":
		return WorkflowReleased
	case "SUPER_USER", "SUPER USER", "SUPERUSER":
		return WorkflowSuperUser
	case "SUBMITTED":
		return WorkflowSubmitted
	case "APPROVED":
		return WorkflowApproved
	default:
		return WorkflowUnspecified
	}
}
