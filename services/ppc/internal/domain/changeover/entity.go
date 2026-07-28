// Package changeover provides domain logic for PPC changeover events — the
// transition period between two work orders on the same machine. Duration and
// waste are component-based (BASE + C1..C7), auto-detected from the product
// difference between the outgoing and incoming WO, with PPC override.
package changeover

import "time"

// Component codes (BASE + C1..C7). Each active component contributes a default
// duration and waste to the changeover estimate. Source: PRD page 6.
const (
	CompBase = "BASE" // base time, always present
	CompC1   = "C1"   // denier change
	CompC2   = "C2"   // color family change
	CompC3   = "C3"   // shade direction (dark -> light)
	CompC4   = "C4"   // filament count change
	CompC5   = "C5"   // twist direction change (S <-> Z)
	CompC6   = "C6"   // lot change (same product)
	CompC7   = "C7"   // deep clean (manual flag)
)

// Status values for a changeover event.
const (
	StatusPlanned    = "PLANNED"
	StatusInProgress = "IN_PROGRESS"
	StatusDone       = "DONE"
)

// Changeover group classification by total estimated duration (minutes).
const (
	GroupMinor  = "MINOR"  // < 60 min
	GroupMedium = "MEDIUM" // 60-120 min
	GroupMajor  = "MAJOR"  // 120-240 min
	GroupDeep   = "DEEP"   // > 240 min
)

// Component is one line of a changeover estimate: a code plus the applied
// duration (minutes) and waste (kg). Auto-detected components carry
// isAutoDetected=true; PPC overrides record the actor and time.
type Component struct {
	id             int64
	eventID        int64
	code           string
	durationMin    int32
	wasteKg        float64
	isAutoDetected bool
	overrideBy     *int64
	overrideAt     *time.Time
}

// NewComponent builds an auto-detected component line.
func NewComponent(code string, durationMin int32, wasteKg float64) Component {
	return Component{code: code, durationMin: durationMin, wasteKg: wasteKg, isAutoDetected: true}
}

// ReconstructComponent rebuilds a Component from persistence (no validation).
func ReconstructComponent(id, eventID int64, code string, durationMin int32, wasteKg float64,
	isAutoDetected bool, overrideBy *int64, overrideAt *time.Time,
) Component {
	return Component{
		id: id, eventID: eventID, code: code, durationMin: durationMin, wasteKg: wasteKg,
		isAutoDetected: isAutoDetected, overrideBy: overrideBy, overrideAt: overrideAt,
	}
}

// ID returns the component id.
func (c Component) ID() int64 { return c.id }

// EventID returns the parent changeover event id.
func (c Component) EventID() int64 { return c.eventID }

// Code returns the component code (BASE / C1..C7).
func (c Component) Code() string { return c.code }

// DurationMin returns the applied duration in minutes.
func (c Component) DurationMin() int32 { return c.durationMin }

// WasteKg returns the applied waste in kilograms.
func (c Component) WasteKg() float64 { return c.wasteKg }

// IsAutoDetected reports whether the component was auto-detected.
func (c Component) IsAutoDetected() bool { return c.isAutoDetected }

// OverrideBy returns the PPC actor id that overrode the component, or nil.
func (c Component) OverrideBy() *int64 { return c.overrideBy }

// OverrideAt returns the override timestamp, or nil.
func (c Component) OverrideAt() *time.Time { return c.overrideAt }

// Event is the aggregate root for a changeover between two WOs on a machine.
// The estimate (duration/waste/group) is derived from its components; actuals
// are captured when the changeover runs.
type Event struct {
	id                int64
	fromWOID          int64
	toWOID            int64
	machineID         int64
	durationEstimated int32
	wasteEstimated    float64
	group             string
	durationActual    *int32
	wasteActual       *float64
	status            string
	startedAt         *time.Time
	completedAt       *time.Time
	notes             string
	components        []Component
}

// NewEvent builds a PLANNED changeover event from its detected components. The
// estimated duration/waste are summed from the components and the group is
// classified from the total duration.
func NewEvent(fromWOID, toWOID, machineID int64, components []Component, notes string) (*Event, error) {
	if fromWOID == 0 || toWOID == 0 {
		return nil, ErrMissingWO
	}
	if machineID == 0 {
		return nil, ErrMissingMachine
	}
	if len(components) == 0 {
		return nil, ErrNoComponents
	}
	dur, waste := sumComponents(components)
	return &Event{
		fromWOID:          fromWOID,
		toWOID:            toWOID,
		machineID:         machineID,
		durationEstimated: dur,
		wasteEstimated:    waste,
		group:             ClassifyGroup(dur),
		status:            StatusPlanned,
		notes:             notes,
		components:        components,
	}, nil
}

// ReconstructEvent rebuilds an Event from persistence (no validation).
func ReconstructEvent(id, fromWOID, toWOID, machineID int64, durationEstimated int32,
	wasteEstimated float64, group string, durationActual *int32, wasteActual *float64,
	status string, startedAt, completedAt *time.Time, notes string, components []Component,
) *Event {
	return &Event{
		id: id, fromWOID: fromWOID, toWOID: toWOID, machineID: machineID,
		durationEstimated: durationEstimated, wasteEstimated: wasteEstimated, group: group,
		durationActual: durationActual, wasteActual: wasteActual, status: status,
		startedAt: startedAt, completedAt: completedAt, notes: notes, components: components,
	}
}

// ID returns the event id.
func (e *Event) ID() int64 { return e.id }

// FromWOID returns the outgoing work order id.
func (e *Event) FromWOID() int64 { return e.fromWOID }

// ToWOID returns the incoming work order id.
func (e *Event) ToWOID() int64 { return e.toWOID }

// MachineID returns the machine id.
func (e *Event) MachineID() int64 { return e.machineID }

// DurationEstimated returns the estimated duration in minutes.
func (e *Event) DurationEstimated() int32 { return e.durationEstimated }

// WasteEstimated returns the estimated waste in kilograms.
func (e *Event) WasteEstimated() float64 { return e.wasteEstimated }

// Group returns the changeover group (MINOR/MEDIUM/MAJOR/DEEP).
func (e *Event) Group() string { return e.group }

// DurationActual returns the actual duration in minutes, or nil.
func (e *Event) DurationActual() *int32 { return e.durationActual }

// WasteActual returns the actual waste in kilograms, or nil.
func (e *Event) WasteActual() *float64 { return e.wasteActual }

// Status returns the changeover status (PLANNED/IN_PROGRESS/DONE).
func (e *Event) Status() string { return e.status }

// StartedAt returns the start timestamp, or nil.
func (e *Event) StartedAt() *time.Time { return e.startedAt }

// CompletedAt returns the completion timestamp, or nil.
func (e *Event) CompletedAt() *time.Time { return e.completedAt }

// Notes returns free-text notes.
func (e *Event) Notes() string { return e.notes }

// Components returns the component breakdown lines.
func (e *Event) Components() []Component { return e.components }

// SetID sets the persisted id after insert (used by the repository).
func (e *Event) SetID(id int64) { e.id = id }

// Start marks the changeover as in-progress and records the start time.
func (e *Event) Start(at time.Time) error {
	if e.status != StatusPlanned {
		return ErrInvalidTransition
	}
	e.status = StatusInProgress
	e.startedAt = &at
	return nil
}

// Complete records the actual duration/waste and marks the changeover done. It
// is valid from PLANNED or IN_PROGRESS (a same-shift changeover may be recorded
// after the fact).
func (e *Event) Complete(durationActual int32, wasteActual float64, at time.Time) error {
	if e.status == StatusDone {
		return ErrInvalidTransition
	}
	if durationActual < 0 || wasteActual < 0 {
		return ErrNegativeActual
	}
	e.durationActual = &durationActual
	e.wasteActual = &wasteActual
	e.completedAt = &at
	e.status = StatusDone
	return nil
}

// sumComponents totals duration and waste across components.
func sumComponents(components []Component) (durationMin int32, wasteKg float64) {
	for i := range components {
		durationMin += components[i].durationMin
		wasteKg += components[i].wasteKg
	}
	return durationMin, wasteKg
}

// ClassifyGroup maps a total estimated duration (minutes) to a changeover group.
func ClassifyGroup(durationMin int32) string {
	switch {
	case durationMin < 60:
		return GroupMinor
	case durationMin <= 120:
		return GroupMedium
	case durationMin <= 240:
		return GroupMajor
	default:
		return GroupDeep
	}
}
