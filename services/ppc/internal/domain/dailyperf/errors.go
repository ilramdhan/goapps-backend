package dailyperf

import "errors"

// Domain errors for daily-performance operations.
var (
	// ErrShiftLogNotFound is returned when a machine shift log is not found.
	ErrShiftLogNotFound = errors.New("machine shift log not found")

	// ErrNoteNotFound is returned when a shift-log note is not found.
	ErrNoteNotFound = errors.New("shift log note not found")

	// ErrInvalidNoteType is returned when a note type is not INSTRUKSI/ACTIVITY.
	ErrInvalidNoteType = errors.New("invalid note type: must be INSTRUKSI or ACTIVITY")

	// ErrInvalidScope is returned when a snapshot scope is not one of the allowed
	// rollup levels.
	ErrInvalidScope = errors.New("invalid scope: must be MACHINE_SHIFT, MACHINE_DAY, or AREA_DAY")

	// ErrInvalidArea is returned when the area code is not one of TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")

	// ErrInvalidShift is returned when the shift code is not 1/2/3.
	ErrInvalidShift = errors.New("invalid shift: must be 1, 2, or 3")

	// ErrInvalidStatus is returned when a shift-log status is not DRAFT/FINAL.
	ErrInvalidStatus = errors.New("invalid status: must be DRAFT or FINAL")

	// ErrInvalidMachine is returned when the machine id is not positive.
	ErrInvalidMachine = errors.New("invalid machine id")

	// ErrEmptyNote is returned when a shift-log note body is empty.
	ErrEmptyNote = errors.New("note cannot be empty")

	// ErrInvalidQty is returned when a quantity is negative.
	ErrInvalidQty = errors.New("invalid quantity: must not be negative")
)
