package workflowtemplate

import "strings"

// Valid kinds.
const (
	KindProductCosting = "PRODUCT_COSTING"
	KindParamFill      = "PARAM_FILL"
)

// Valid approver resolution types.
const (
	ResolutionRole = "ROLE"
	ResolutionUser = "USER"
	ResolutionDept = "DEPT"
)

// Kind is the discriminator for use cases.
type Kind struct{ value string }

// NewKind validates and returns a Kind.
func NewKind(s string) (Kind, error) {
	switch strings.TrimSpace(s) {
	case KindProductCosting, KindParamFill:
		return Kind{value: s}, nil
	default:
		return Kind{}, ErrInvalidKind
	}
}

// String returns the underlying kind.
func (k Kind) String() string { return k.value }

// Name is the display name.
type Name struct{ value string }

// NewName validates and returns a Name.
func NewName(s string) (Name, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || len(s) > 200 {
		return Name{}, ErrInvalidName
	}
	return Name{value: s}, nil
}

// String returns the underlying name.
func (n Name) String() string { return n.value }

// Description is the optional template description.
type Description struct{ value string }

// NewDescription validates and returns a Description.
func NewDescription(s string) (Description, error) {
	if len(s) > 1000 {
		return Description{}, ErrInvalidDesc
	}
	return Description{value: s}, nil
}

// String returns the underlying description.
func (d Description) String() string { return d.value }

// Resolution is the approver resolution type.
type Resolution struct{ value string }

// NewResolution validates and returns a Resolution.
func NewResolution(s string) (Resolution, error) {
	switch strings.TrimSpace(s) {
	case ResolutionRole, ResolutionUser, ResolutionDept:
		return Resolution{value: s}, nil
	default:
		return Resolution{}, ErrInvalidResolution
	}
}

// String returns the underlying resolution type.
func (r Resolution) String() string { return r.value }
