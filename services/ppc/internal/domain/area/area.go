// Package area provides the shared Area value object (production area code).
package area

import (
	"errors"
	"strings"
)

// ErrInvalid is returned when an area code is not one of the allowed values.
var ErrInvalid = errors.New("invalid area: must be TXT, SPG, or TWT")

// Area is a validated production-area code value object (TXT/SPG/TWT).
type Area struct {
	value string
}

// New creates a validated Area from a string.
func New(s string) (Area, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "TXT", "SPG", "TWT":
		return Area{value: s}, nil
	default:
		return Area{}, ErrInvalid
	}
}

// String returns the string representation of the area.
func (a Area) String() string { return a.value }

// IsEmpty returns true if the area is unset.
func (a Area) IsEmpty() bool { return a.value == "" }
