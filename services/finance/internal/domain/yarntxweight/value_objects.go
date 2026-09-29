// Package yarntxweight provides domain logic for the global TX Weight master
// (legacy CST_YARN_TX_WEIGHT): one weight rule per product type x grade.
package yarntxweight

import "strings"

// Grade is a yarn grade that carries a TX Weight rule.
type Grade string

// Supported grades.
const (
	GradeAE Grade = "AE"
	GradeA9 Grade = "A9"
	GradeA  Grade = "A"
	GradeB  Grade = "B"
	GradeC  Grade = "C"
)

// AllGrades lists every supported grade in display order.
var AllGrades = []Grade{GradeAE, GradeA9, GradeA, GradeB, GradeC}

// ParseGrade validates and normalizes a grade string.
func ParseGrade(s string) (Grade, error) {
	g := Grade(strings.ToUpper(strings.TrimSpace(s)))
	switch g {
	case GradeAE, GradeA9, GradeA, GradeB, GradeC:
		return g, nil
	default:
		return "", ErrInvalidGrade
	}
}

// String returns the grade code.
func (g Grade) String() string { return string(g) }

// Mode is how a TX Weight rule derives the grade weight from AX_WT.
type Mode string

// Supported modes.
const (
	// ModeLessBy yields AX_WT - value.
	ModeLessBy Mode = "LESS_BY"
	// ModeMultiply yields AX_WT * value.
	ModeMultiply Mode = "MULTIPLY"
	// ModeFixed yields value regardless of AX_WT.
	ModeFixed Mode = "FIXED"
)

// ParseMode validates and normalizes a mode string.
func ParseMode(s string) (Mode, error) {
	m := Mode(strings.ToUpper(strings.TrimSpace(s)))
	switch m {
	case ModeLessBy, ModeMultiply, ModeFixed:
		return m, nil
	default:
		return "", ErrInvalidMode
	}
}

// String returns the mode code.
func (m Mode) String() string { return string(m) }

// Apply computes the grade weight for axWt under this mode and value.
func (m Mode) Apply(axWt, value float64) float64 {
	switch m {
	case ModeLessBy:
		return axWt - value
	case ModeMultiply:
		return axWt * value
	case ModeFixed:
		return value
	default:
		return axWt
	}
}
