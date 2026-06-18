// Package intermingling provides domain logic for Intermingling cost lookup management.
package intermingling

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the Intermingling domain.
type Entity struct {
	id        uuid.UUID
	code      string
	name      string
	costPerKg float64
	isActive  bool
	notes     string
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
	deletedAt *time.Time
	deletedBy *string
}

// New creates a new Intermingling entity with validation.
func New(code, name string, costPerKg float64, notes, createdBy string) (*Entity, error) {
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if costPerKg < 0 {
		return nil, ErrInvalidCost
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), code: code, name: name, costPerKg: costPerKg,
		isActive: true, notes: notes, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds an Intermingling entity from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, code, name string, costPerKg float64, isActive bool, notes string, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, code: code, name: name, costPerKg: costPerKg, isActive: isActive, notes: notes,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// Code returns the intermingling code (e.g. HIM, SIM).
func (e *Entity) Code() string { return e.code }

// Name returns the display name.
func (e *Entity) Name() string { return e.name }

// CostPerKg returns the cost in USD/kg.
func (e *Entity) CostPerKg() float64 { return e.costPerKg }

// IsActive returns whether the record is active.
func (e *Entity) IsActive() bool { return e.isActive }

// Notes returns optional notes.
func (e *Entity) Notes() string { return e.notes }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() time.Time { return e.createdAt }

// CreatedBy returns the creator.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() *time.Time { return e.updatedAt }

// UpdatedBy returns the last updater.
func (e *Entity) UpdatedBy() *string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() *time.Time { return e.deletedAt }

// DeletedBy returns who soft-deleted the record.
func (e *Entity) DeletedBy() *string { return e.deletedBy }

// IsDeleted returns true if the record is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	Name      *string
	CostPerKg *float64
	IsActive  *bool
	Notes     *string
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyName(in.Name); err != nil {
		return err
	}
	if err := e.applyCostPerKg(in.CostPerKg); err != nil {
		return err
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
	if in.Notes != nil {
		e.notes = *in.Notes
	}
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the record as deleted.
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

func (e *Entity) applyCostPerKg(cost *float64) error {
	if cost == nil {
		return nil
	}
	if *cost < 0 {
		return ErrInvalidCost
	}
	e.costPerKg = *cost
	return nil
}

func validateCode(code string) error {
	if code == "" {
		return ErrEmptyCode
	}
	if len(code) > 20 {
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
