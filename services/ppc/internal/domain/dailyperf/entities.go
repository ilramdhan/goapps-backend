package dailyperf

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
)

// isValidShift reports whether s is an allowed shift code (1/2/3).
func isValidShift(s string) bool {
	switch s {
	case "1", "2", "3":
		return true
	default:
		return false
	}
}

// ── MachineShiftLog ──────────────────────────────────────────────────────────

// MachineShiftLog is the per-machine, per-shift record feeding the efficiency
// engine. running_minutes is DERIVED from downtime (v1.2), not typed by the
// operator.
type MachineShiftLog struct {
	id               int64
	machineID        int64
	machineNo        string
	date             time.Time
	shift            string
	positionsTotal   int32
	positionsRunning float64
	runningMinutes   int32
	status           string
	inputBy          int64
	inputAt          time.Time
	updatedAt        time.Time
}

// NewShiftLogParams carries the inputs for constructing a MachineShiftLog.
type NewShiftLogParams struct {
	MachineID        int64
	Date             time.Time
	Shift            string
	PositionsTotal   int32
	PositionsRunning float64
	RunningMinutes   int32
	Status           string
	InputBy          int64
}

// NewMachineShiftLog creates a validated machine shift log.
func NewMachineShiftLog(p NewShiftLogParams) (*MachineShiftLog, error) {
	if p.MachineID <= 0 {
		return nil, ErrInvalidMachine
	}
	if !isValidShift(p.Shift) {
		return nil, ErrInvalidShift
	}
	if !IsValidStatus(p.Status) {
		return nil, ErrInvalidStatus
	}
	status := p.Status
	if status == "" {
		status = StatusDraft
	}
	now := time.Now()
	return &MachineShiftLog{
		machineID:        p.MachineID,
		date:             p.Date,
		shift:            p.Shift,
		positionsTotal:   p.PositionsTotal,
		positionsRunning: p.PositionsRunning,
		runningMinutes:   p.RunningMinutes,
		status:           status,
		inputBy:          p.InputBy,
		inputAt:          now,
		updatedAt:        now,
	}, nil
}

// ReconstructShiftLogParams carries persisted fields for rebuilding a shift log.
type ReconstructShiftLogParams struct {
	ID               int64
	MachineID        int64
	MachineNo        string
	Date             time.Time
	Shift            string
	PositionsTotal   int32
	PositionsRunning float64
	RunningMinutes   int32
	Status           string
	InputBy          int64
	InputAt          time.Time
	UpdatedAt        time.Time
}

// ReconstructMachineShiftLog rebuilds a MachineShiftLog from persistence.
func ReconstructMachineShiftLog(p ReconstructShiftLogParams) *MachineShiftLog {
	return &MachineShiftLog{
		id:               p.ID,
		machineID:        p.MachineID,
		machineNo:        p.MachineNo,
		date:             p.Date,
		shift:            p.Shift,
		positionsTotal:   p.PositionsTotal,
		positionsRunning: p.PositionsRunning,
		runningMinutes:   p.RunningMinutes,
		status:           p.Status,
		inputBy:          p.InputBy,
		inputAt:          p.InputAt,
		updatedAt:        p.UpdatedAt,
	}
}

// ID returns the shift-log identifier.
func (m *MachineShiftLog) ID() int64 { return m.id }

// MachineID returns the machine id.
func (m *MachineShiftLog) MachineID() int64 { return m.machineID }

// MachineNo returns the denormalized machine number.
func (m *MachineShiftLog) MachineNo() string { return m.machineNo }

// Date returns the production date.
func (m *MachineShiftLog) Date() time.Time { return m.date }

// Shift returns the shift code (1/2/3).
func (m *MachineShiftLog) Shift() string { return m.shift }

// PositionsTotal returns the total installed positions.
func (m *MachineShiftLog) PositionsTotal() int32 { return m.positionsTotal }

// PositionsRunning returns the running positions (may be fractional).
func (m *MachineShiftLog) PositionsRunning() float64 { return m.positionsRunning }

// RunningMinutes returns the derived running minutes.
func (m *MachineShiftLog) RunningMinutes() int32 { return m.runningMinutes }

// Status returns the shift-log status (DRAFT/FINAL).
func (m *MachineShiftLog) Status() string { return m.status }

// InputBy returns the recording user id.
func (m *MachineShiftLog) InputBy() int64 { return m.inputBy }

// InputAt returns the recording timestamp.
func (m *MachineShiftLog) InputAt() time.Time { return m.inputAt }

// UpdatedAt returns the last-update timestamp.
func (m *MachineShiftLog) UpdatedAt() time.Time { return m.updatedAt }

// SetID assigns the generated id (used by the repository after upsert).
func (m *MachineShiftLog) SetID(id int64) { m.id = id }

// SetMachineNo sets the denormalized machine number (used by the repository).
func (m *MachineShiftLog) SetMachineNo(no string) { m.machineNo = no }

// ── AreaShiftLog ─────────────────────────────────────────────────────────────

// AreaShiftLog is the per-area overtime + notes record; a nil shift means the
// row is a daily (not per-shift) entry.
type AreaShiftLog struct {
	id      int64
	area    area.Area
	date    time.Time
	shift   *string
	otHours *float64
	notes   string
	inputBy int64
	inputAt time.Time
}

// NewAreaShiftLogParams carries the inputs for constructing an AreaShiftLog.
type NewAreaShiftLogParams struct {
	Area    string
	Date    time.Time
	Shift   *string
	OtHours *float64
	Notes   string
	InputBy int64
}

// NewAreaShiftLog creates a validated area shift log.
func NewAreaShiftLog(p NewAreaShiftLogParams) (*AreaShiftLog, error) {
	ac, err := area.New(p.Area)
	if err != nil {
		return nil, ErrInvalidArea
	}
	if p.Shift != nil && !isValidShift(*p.Shift) {
		return nil, ErrInvalidShift
	}
	return &AreaShiftLog{
		area:    ac,
		date:    p.Date,
		shift:   p.Shift,
		otHours: p.OtHours,
		notes:   p.Notes,
		inputBy: p.InputBy,
		inputAt: time.Now(),
	}, nil
}

// ReconstructAreaShiftLog rebuilds an AreaShiftLog from persistence. The area
// code was validated on write, so a parse failure yields the zero Area.
func ReconstructAreaShiftLog(id int64, areaCode string, date time.Time, shift *string, otHours *float64, notes string, inputBy int64, inputAt time.Time) *AreaShiftLog {
	ac, err := area.New(areaCode)
	if err != nil {
		ac = area.Area{}
	}
	return &AreaShiftLog{
		id:      id,
		area:    ac,
		date:    date,
		shift:   shift,
		otHours: otHours,
		notes:   notes,
		inputBy: inputBy,
		inputAt: inputAt,
	}
}

// ID returns the area-shift-log identifier.
func (a *AreaShiftLog) ID() int64 { return a.id }

// AreaCode returns the production area code.
func (a *AreaShiftLog) AreaCode() string { return a.area.String() }

// Date returns the production date.
func (a *AreaShiftLog) Date() time.Time { return a.date }

// Shift returns the optional shift code (nil = daily).
func (a *AreaShiftLog) Shift() *string { return a.shift }

// OtHours returns the optional overtime hours.
func (a *AreaShiftLog) OtHours() *float64 { return a.otHours }

// Notes returns the free-text notes.
func (a *AreaShiftLog) Notes() string { return a.notes }

// InputBy returns the recording user id.
func (a *AreaShiftLog) InputBy() int64 { return a.inputBy }

// InputAt returns the recording timestamp.
func (a *AreaShiftLog) InputAt() time.Time { return a.inputAt }

// SetID assigns the generated id (used by the repository after upsert).
func (a *AreaShiftLog) SetID(id int64) { a.id = id }

// ── DowntimeEvent ────────────────────────────────────────────────────────────

// DowntimeEvent is one downtime occurrence on a machine (optionally a specific
// position) within a shift, referencing a downtime reason.
type DowntimeEvent struct {
	ID          int64
	MachineID   int64
	MachineNo   string
	WoID        *int64
	ShiftLogID  *int64
	CeID        *int64
	Date        time.Time
	Shift       string
	PositionNo  *string
	ReasonID    int64
	ReasonCode  string
	StartAt     *time.Time
	EndAt       *time.Time
	DurationMin *int32
	LostKg      *float64
	Notes       *string
	InputBy     int64
	InputAt     time.Time
}

// ── WasteActual ──────────────────────────────────────────────────────────────

// WasteActual is one recorded waste quantity, optionally tied to a machine/WO.
type WasteActual struct {
	ID         int64
	Area       string
	MachineID  *int64
	WoID       *int64
	ShiftLogID *int64
	Date       time.Time
	Shift      string
	CategoryID int64
	QtyKg      float64
	IsUpset    bool
	Notes      *string
	InputBy    int64
	InputAt    time.Time
}

// ── EfficiencySnapshot ───────────────────────────────────────────────────────

// EfficiencySnapshot is a computed efficiency row for a scope (MACHINE_SHIFT →
// MACHINE_DAY → AREA_DAY), in an Including or Excluding variant.
type EfficiencySnapshot struct {
	ID                int64
	Area              string
	Scope             string
	MachineID         *int64
	WoID              *int64
	Date              time.Time
	Shift             *string
	Segment           *string
	IsExcluding       bool
	QtyTheoretical100 float64
	QtyTheoreticalRng float64
	QtyLoss           float64
	QtyWaste          float64
	QtyActual         float64
	EffProductionPct  float64
	EffRunningPct     float64
	EffPlantPct       float64
	YieldPct          float64
	WastePct          float64
	BreaksCount       int32
	BreaksPerTon      float64
	CalcAt            time.Time
}

// ── ShiftLogNote ─────────────────────────────────────────────────────────────

// ShiftLogNote is a shift-log book entry (INSTRUKSI/ACTIVITY) for a machine on a
// date+shift; replaces the removed msl_activity_note (v1.2).
type ShiftLogNote struct {
	id        int64
	machineID int64
	machineNo string
	date      time.Time
	shift     string
	noteType  NoteType
	note      string
	woID      *int64
	inputBy   int64
	inputAt   time.Time
}

// NewShiftLogNoteParams carries the inputs for constructing a ShiftLogNote.
type NewShiftLogNoteParams struct {
	MachineID int64
	Date      time.Time
	Shift     string
	NoteType  string
	Note      string
	WoID      *int64
	InputBy   int64
}

// NewShiftLogNote creates a validated shift-log note.
func NewShiftLogNote(p NewShiftLogNoteParams) (*ShiftLogNote, error) {
	if p.MachineID <= 0 {
		return nil, ErrInvalidMachine
	}
	if !isValidShift(p.Shift) {
		return nil, ErrInvalidShift
	}
	nt, err := NewNoteType(p.NoteType)
	if err != nil {
		return nil, err
	}
	if p.Note == "" {
		return nil, ErrEmptyNote
	}
	return &ShiftLogNote{
		machineID: p.MachineID,
		date:      p.Date,
		shift:     p.Shift,
		noteType:  nt,
		note:      p.Note,
		woID:      p.WoID,
		inputBy:   p.InputBy,
		inputAt:   time.Now(),
	}, nil
}

// ReconstructShiftLogNoteParams carries persisted fields for a ShiftLogNote.
type ReconstructShiftLogNoteParams struct {
	ID        int64
	MachineID int64
	MachineNo string
	Date      time.Time
	Shift     string
	NoteType  string
	Note      string
	WoID      *int64
	InputBy   int64
	InputAt   time.Time
}

// ReconstructShiftLogNote rebuilds a ShiftLogNote from persistence. The note type
// was validated on write, so a parse failure yields the zero NoteType.
func ReconstructShiftLogNote(p ReconstructShiftLogNoteParams) *ShiftLogNote {
	nt, err := NewNoteType(p.NoteType)
	if err != nil {
		nt = NoteType{}
	}
	return &ShiftLogNote{
		id:        p.ID,
		machineID: p.MachineID,
		machineNo: p.MachineNo,
		date:      p.Date,
		shift:     p.Shift,
		noteType:  nt,
		note:      p.Note,
		woID:      p.WoID,
		inputBy:   p.InputBy,
		inputAt:   p.InputAt,
	}
}

// Update applies optional note-type and body changes with validation.
func (n *ShiftLogNote) Update(noteType, note *string) error {
	if noteType != nil {
		nt, err := NewNoteType(*noteType)
		if err != nil {
			return err
		}
		n.noteType = nt
	}
	if note != nil {
		if *note == "" {
			return ErrEmptyNote
		}
		n.note = *note
	}
	return nil
}

// ID returns the note identifier.
func (n *ShiftLogNote) ID() int64 { return n.id }

// MachineID returns the machine id.
func (n *ShiftLogNote) MachineID() int64 { return n.machineID }

// MachineNo returns the denormalized machine number.
func (n *ShiftLogNote) MachineNo() string { return n.machineNo }

// Date returns the note date.
func (n *ShiftLogNote) Date() time.Time { return n.date }

// Shift returns the shift code (1/2/3).
func (n *ShiftLogNote) Shift() string { return n.shift }

// NoteType returns the note type (INSTRUKSI/ACTIVITY).
func (n *ShiftLogNote) NoteType() string { return n.noteType.String() }

// Note returns the note body.
func (n *ShiftLogNote) Note() string { return n.note }

// WoID returns the optional work-order soft reference.
func (n *ShiftLogNote) WoID() *int64 { return n.woID }

// InputBy returns the recording user id.
func (n *ShiftLogNote) InputBy() int64 { return n.inputBy }

// InputAt returns the recording timestamp.
func (n *ShiftLogNote) InputAt() time.Time { return n.inputAt }

// SetID assigns the generated id (used by the repository after insert).
func (n *ShiftLogNote) SetID(id int64) { n.id = id }

// SetMachineNo sets the denormalized machine number (used by the repository).
func (n *ShiftLogNote) SetMachineNo(no string) { n.machineNo = no }
