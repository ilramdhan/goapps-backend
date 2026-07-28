package dailyperf

import "strings"

// Shift-log note types (v1.2 shift log book).
const (
	// NoteInstruksi is a shift-handover instruction note.
	NoteInstruksi = "INSTRUKSI"
	// NoteActivity is an activity-log note.
	NoteActivity = "ACTIVITY"
)

// NoteType is a validated shift-log note type value object (INSTRUKSI/ACTIVITY).
type NoteType struct {
	value string
}

// NewNoteType creates a validated NoteType from a string.
func NewNoteType(s string) (NoteType, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case NoteInstruksi:
		return NoteType{value: NoteInstruksi}, nil
	case NoteActivity:
		return NoteType{value: NoteActivity}, nil
	default:
		return NoteType{}, ErrInvalidNoteType
	}
}

// String returns the string representation of the note type.
func (n NoteType) String() string { return n.value }

// Efficiency snapshot scopes (rollup levels).
const (
	// ScopeMachineShift is a per-machine, per-shift snapshot.
	ScopeMachineShift = "MACHINE_SHIFT"
	// ScopeMachineDay is a per-machine daily rollup over shifts.
	ScopeMachineDay = "MACHINE_DAY"
	// ScopeAreaDay is a per-area daily rollup over machines.
	ScopeAreaDay = "AREA_DAY"
)

// Scope is a validated efficiency-snapshot scope value object.
type Scope struct {
	value string
}

// NewScope creates a validated Scope from a string.
func NewScope(s string) (Scope, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case ScopeMachineShift:
		return Scope{value: ScopeMachineShift}, nil
	case ScopeMachineDay:
		return Scope{value: ScopeMachineDay}, nil
	case ScopeAreaDay:
		return Scope{value: ScopeAreaDay}, nil
	default:
		return Scope{}, ErrInvalidScope
	}
}

// String returns the string representation of the scope.
func (s Scope) String() string { return s.value }

// Efficiency segments (product families within an area).
const (
	// SegmentDTY is the draw-textured yarn segment (TXT).
	SegmentDTY = "DTY"
	// SegmentACY is the air-covered yarn segment (TXT).
	SegmentACY = "ACY"
	// SegmentATY is the air-textured yarn segment (TXT).
	SegmentATY = "ATY"
	// SegmentPOY is the partially-oriented yarn segment (SPG).
	SegmentPOY = "POY"
)

// Statuses for a machine shift log (no approval gate in v1.2).
const (
	// StatusDraft marks a shift log still being edited.
	StatusDraft = "DRAFT"
	// StatusFinal marks a finalized shift log.
	StatusFinal = "FINAL"
)

// IsValidStatus reports whether s is an allowed machine-shift-log status. An
// empty status is allowed and defaults to DRAFT at construction.
func IsValidStatus(s string) bool {
	switch s {
	case "", StatusDraft, StatusFinal:
		return true
	default:
		return false
	}
}
