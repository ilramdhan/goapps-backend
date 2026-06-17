// Package machine provides domain logic for Machine master data management.
package machine

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the Machine domain.
type Entity struct {
	id           uuid.UUID
	code         string
	name         string
	mcType       string
	location     string
	noOfPosition int
	noOfEnd      int
	mcSpeed      float64
	machineRPM   *float64
	mcEfficiency float64
	powerPerDay  *float64
	isActive     bool
	notes        string
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
	deletedAt    *time.Time
	deletedBy    *string
}

// New creates a new Machine entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(code, name, mcType, location string, noOfPosition, noOfEnd int, mcSpeed float64, machineRPM *float64, mcEfficiency float64, powerPerDay *float64, notes, createdBy string) (*Entity, error) {
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), code: code, name: name, mcType: mcType, location: location,
		noOfPosition: noOfPosition, noOfEnd: noOfEnd, mcSpeed: mcSpeed, machineRPM: machineRPM,
		mcEfficiency: mcEfficiency, powerPerDay: powerPerDay, isActive: true, notes: notes,
		createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds a Machine from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, code, name, mcType, location string, noOfPosition, noOfEnd int, mcSpeed float64, machineRPM *float64, mcEfficiency float64, powerPerDay *float64, isActive bool, notes string, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, code: code, name: name, mcType: mcType, location: location,
		noOfPosition: noOfPosition, noOfEnd: noOfEnd, mcSpeed: mcSpeed, machineRPM: machineRPM,
		mcEfficiency: mcEfficiency, powerPerDay: powerPerDay, isActive: isActive, notes: notes,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the machine UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// Code returns the machine code.
func (e *Entity) Code() string { return e.code }

// Name returns the machine name.
func (e *Entity) Name() string { return e.name }

// MCType returns the machine type (DTY, POY, PTY, FDY, etc.).
func (e *Entity) MCType() string { return e.mcType }

// Location returns the machine location.
func (e *Entity) Location() string { return e.location }

// NoOfPosition returns the number of positions.
func (e *Entity) NoOfPosition() int { return e.noOfPosition }

// NoOfEnd returns the number of ends.
func (e *Entity) NoOfEnd() int { return e.noOfEnd }

// MCSpeed returns the machine speed in m/min.
func (e *Entity) MCSpeed() float64 { return e.mcSpeed }

// MachineRPM returns the optional machine RPM.
func (e *Entity) MachineRPM() *float64 { return e.machineRPM }

// MCEfficiency returns the machine efficiency percentage.
func (e *Entity) MCEfficiency() float64 { return e.mcEfficiency }

// PowerPerDay returns the optional power cost per day in USD.
func (e *Entity) PowerPerDay() *float64 { return e.powerPerDay }

// IsActive returns whether the machine is active.
func (e *Entity) IsActive() bool { return e.isActive }

// Notes returns optional notes.
func (e *Entity) Notes() string { return e.notes }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() time.Time { return e.createdAt }

// CreatedBy returns the creator identifier.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() *time.Time { return e.updatedAt }

// UpdatedBy returns the last updater identifier.
func (e *Entity) UpdatedBy() *string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() *time.Time { return e.deletedAt }

// DeletedBy returns who soft-deleted the record.
func (e *Entity) DeletedBy() *string { return e.deletedBy }

// IsDeleted returns true if the machine has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	Name         *string
	MCType       *string
	Location     *string
	NoOfPosition *int
	NoOfEnd      *int
	MCSpeed      *float64
	MachineRPM   *float64
	MCEfficiency *float64
	PowerPerDay  *float64
	IsActive     *bool
	Notes        *string
}

// Update applies optional field changes to the machine entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyName(in.Name); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the machine as deleted.
func (e *Entity) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	now := time.Now()
	e.deletedAt = &now
	e.deletedBy = &deletedBy
	e.isActive = false
	return nil
}

func (e *Entity) applyName(name *string) error {
	if name == nil {
		return nil
	}
	if err := validateName(*name); err != nil {
		return err
	}
	e.name = *name
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.MCType != nil {
		e.mcType = *in.MCType
	}
	if in.Location != nil {
		e.location = *in.Location
	}
	if in.NoOfPosition != nil {
		e.noOfPosition = *in.NoOfPosition
	}
	if in.NoOfEnd != nil {
		e.noOfEnd = *in.NoOfEnd
	}
	if in.MCSpeed != nil {
		e.mcSpeed = *in.MCSpeed
	}
	if in.MachineRPM != nil {
		e.machineRPM = in.MachineRPM
	}
	if in.MCEfficiency != nil {
		e.mcEfficiency = *in.MCEfficiency
	}
	if in.PowerPerDay != nil {
		e.powerPerDay = in.PowerPerDay
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
	if in.Notes != nil {
		e.notes = *in.Notes
	}
}

func validateCode(code string) error {
	if code == "" {
		return ErrEmptyCode
	}
	if len(code) > 30 {
		return ErrCodeTooLong
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return ErrEmptyName
	}
	if len(name) > 100 {
		return ErrNameTooLong
	}
	return nil
}
