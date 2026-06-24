// Package productgrade provides domain logic for Product Grade quality-loss configuration.
package productgrade

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the Product Grade domain.
type Entity struct {
	id              uuid.UUID
	code            string
	name            string
	description     string
	bcPerc          float64
	nonStdPerc      float64
	bcRecoveryRate  float64
	pgDetailProduct string
	pgGradeLabel    string
	stdSellingPrice float64
	spValue         float64
	isActive        bool
	notes           string
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
	deletedAt       *time.Time
	deletedBy       *string
}

// New creates a new Product Grade entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(code, name, description string, bcPerc, nonStdPerc, bcRecoveryRate float64, pgDetailProduct, pgGradeLabel string, stdSellingPrice, spValue float64, notes, createdBy string) (*Entity, error) {
	if code == "" {
		return nil, ErrEmptyCode
	}
	if len(code) > 30 {
		return nil, ErrCodeTooLong
	}
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 100 {
		return nil, ErrNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), code: code, name: name, description: description,
		bcPerc: bcPerc, nonStdPerc: nonStdPerc, bcRecoveryRate: bcRecoveryRate,
		pgDetailProduct: pgDetailProduct, pgGradeLabel: pgGradeLabel,
		stdSellingPrice: stdSellingPrice, spValue: spValue,
		isActive: true, notes: notes, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds a Product Grade from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, code, name, description string, bcPerc, nonStdPerc, bcRecoveryRate float64, pgDetailProduct, pgGradeLabel string, stdSellingPrice, spValue float64, isActive bool, notes string, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, code: code, name: name, description: description,
		bcPerc: bcPerc, nonStdPerc: nonStdPerc, bcRecoveryRate: bcRecoveryRate,
		pgDetailProduct: pgDetailProduct, pgGradeLabel: pgGradeLabel,
		stdSellingPrice: stdSellingPrice, spValue: spValue,
		isActive: isActive, notes: notes, createdAt: createdAt, createdBy: createdBy,
		updatedAt: updatedAt, updatedBy: updatedBy, deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// Code returns the grade code.
func (e *Entity) Code() string { return e.code }

// Name returns the display name.
func (e *Entity) Name() string { return e.name }

// Description returns the grade description.
func (e *Entity) Description() string { return e.description }

// BCPerc returns BC output percentage.
func (e *Entity) BCPerc() float64 { return e.bcPerc }

// NonStdPerc returns Non-Standard output percentage.
func (e *Entity) NonStdPerc() float64 { return e.nonStdPerc }

// BCRecoveryRate returns BC value recovery rate percentage.
func (e *Entity) BCRecoveryRate() float64 { return e.bcRecoveryRate }

// PgDetailProduct returns the Oracle CMPG_DETAIL_PRODUCT pattern key.
func (e *Entity) PgDetailProduct() string { return e.pgDetailProduct }

// PgGradeLabel returns the grade label for STD_VALUE_LOSS.
func (e *Entity) PgGradeLabel() string { return e.pgGradeLabel }

// StdSellingPrice returns the BC_SPECIAL_PROD rate.
func (e *Entity) StdSellingPrice() float64 { return e.stdSellingPrice }

// SpValue returns the VALUE_LOSS rate.
func (e *Entity) SpValue() float64 { return e.spValue }

// IsActive returns whether the grade is active.
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

// IsDeleted returns true if the grade is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	Name            *string
	Description     *string
	BCPerc          *float64
	NonStdPerc      *float64
	BCRecoveryRate  *float64
	PgDetailProduct *string
	PgGradeLabel    *string
	StdSellingPrice *float64
	SpValue         *float64
	IsActive        *bool
	Notes           *string
}

// Update applies optional field changes to the entity.
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

// SoftDelete marks the grade as deleted.
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
	if *name == "" {
		return ErrEmptyName
	}
	if len(*name) > 100 {
		return ErrNameTooLong
	}
	e.name = *name
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.Description != nil {
		e.description = *in.Description
	}
	if in.BCPerc != nil {
		e.bcPerc = *in.BCPerc
	}
	if in.NonStdPerc != nil {
		e.nonStdPerc = *in.NonStdPerc
	}
	if in.BCRecoveryRate != nil {
		e.bcRecoveryRate = *in.BCRecoveryRate
	}
	if in.PgDetailProduct != nil {
		e.pgDetailProduct = *in.PgDetailProduct
	}
	if in.PgGradeLabel != nil {
		e.pgGradeLabel = *in.PgGradeLabel
	}
	if in.StdSellingPrice != nil {
		e.stdSellingPrice = *in.StdSellingPrice
	}
	if in.SpValue != nil {
		e.spValue = *in.SpValue
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
	if in.Notes != nil {
		e.notes = *in.Notes
	}
}
