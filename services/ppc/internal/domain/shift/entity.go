// Package shift provides domain logic for the PPC shift master. A shift is a
// genuine per-row master (code plus start/end time). end_time may be earlier
// than start_time to model a shift crossing midnight (e.g. shift 3: 22:00-06:00).
package shift

import (
	"regexp"
	"time"
)

const maxNameLen = 40

// timeHHMM matches a 24-hour HH:MM time-of-day.
var timeHHMM = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// codePattern matches a single digit shift code.
var codePattern = regexp.MustCompile(`^[0-9]$`)

// Shift is the aggregate root for the shift master.
type Shift struct {
	id        int64
	code      string
	name      string
	startTime string // HH:MM
	endTime   string // HH:MM
	isActive  bool
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
}

// NewShift creates a new Shift with validation.
func NewShift(code, name, startTime, endTime, createdBy string) (*Shift, error) {
	if !codePattern.MatchString(code) {
		return nil, ErrInvalidCode
	}
	if len(name) > maxNameLen {
		return nil, ErrNameTooLong
	}
	if !timeHHMM.MatchString(startTime) {
		return nil, ErrInvalidStartTime
	}
	if !timeHHMM.MatchString(endTime) {
		return nil, ErrInvalidEndTime
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Shift{
		code:      code,
		name:      name,
		startTime: startTime,
		endTime:   endTime,
		isActive:  true,
		createdAt: time.Now(),
		createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds a Shift from persistence (no validation).
func Reconstruct(
	id int64,
	code, name, startTime, endTime string,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Shift {
	return &Shift{
		id:        id,
		code:      code,
		name:      name,
		startTime: startTime,
		endTime:   endTime,
		isActive:  isActive,
		createdAt: createdAt,
		createdBy: createdBy,
		updatedAt: updatedAt,
		updatedBy: updatedBy,
	}
}

// ID returns the shift identifier.
func (s *Shift) ID() int64 { return s.id }

// Code returns the shift code.
func (s *Shift) Code() string { return s.code }

// Name returns the shift name.
func (s *Shift) Name() string { return s.name }

// StartTime returns the shift start time as HH:MM.
func (s *Shift) StartTime() string { return s.startTime }

// EndTime returns the shift end time as HH:MM.
func (s *Shift) EndTime() string { return s.endTime }

// IsActive returns whether the shift is active.
func (s *Shift) IsActive() bool { return s.isActive }

// CreatedAt returns the creation timestamp.
func (s *Shift) CreatedAt() time.Time { return s.createdAt }

// CreatedBy returns the creator.
func (s *Shift) CreatedBy() string { return s.createdBy }

// UpdatedAt returns the last update timestamp.
func (s *Shift) UpdatedAt() *time.Time { return s.updatedAt }

// UpdatedBy returns the last updater.
func (s *Shift) UpdatedBy() *string { return s.updatedBy }

// Update applies optional field changes with validation. Code is immutable.
func (s *Shift) Update(name, startTime, endTime *string, isActive *bool, updatedBy string) error {
	if name != nil {
		if len(*name) > maxNameLen {
			return ErrNameTooLong
		}
		s.name = *name
	}
	if startTime != nil {
		if !timeHHMM.MatchString(*startTime) {
			return ErrInvalidStartTime
		}
		s.startTime = *startTime
	}
	if endTime != nil {
		if !timeHHMM.MatchString(*endTime) {
			return ErrInvalidEndTime
		}
		s.endTime = *endTime
	}
	if isActive != nil {
		s.isActive = *isActive
	}

	now := time.Now()
	s.updatedAt = &now
	s.updatedBy = &updatedBy
	return nil
}
