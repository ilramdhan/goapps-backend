// Package yarntxweight provides domain logic for the global TX Weight master.
package yarntxweight

import (
	"math"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxDescriptionLen mirrors mst_yarn_tx_weight.ytw_description VARCHAR(200).
const maxDescriptionLen = 200

// Entity is the aggregate root for a TX Weight rule.
type Entity struct {
	id              uuid.UUID
	productTypeID   int32
	productTypeCode string
	productTypeName string
	grade           Grade
	mode            Mode
	value           float64
	description     string
	oracleSysID     string
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
	deletedAt       *time.Time
	deletedBy       *string
}

// New creates a new TX Weight rule with validation.
func New(productTypeID int32, grade Grade, mode Mode, value float64, description, createdBy string) (*Entity, error) {
	if productTypeID <= 0 {
		return nil, ErrInvalidProductType
	}
	if _, err := ParseGrade(string(grade)); err != nil {
		return nil, err
	}
	if _, err := ParseMode(string(mode)); err != nil {
		return nil, err
	}
	if err := validateValue(value); err != nil {
		return nil, err
	}
	if err := validateDescription(description); err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), productTypeID: productTypeID, grade: grade, mode: mode,
		value: value, description: description, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// ReconstructParams carries persisted fields for Reconstruct.
type ReconstructParams struct {
	ID              uuid.UUID
	ProductTypeID   int32
	ProductTypeCode string
	ProductTypeName string
	Grade           Grade
	Mode            Mode
	Value           float64
	Description     string
	OracleSysID     string
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       *time.Time
	UpdatedBy       *string
	DeletedAt       *time.Time
	DeletedBy       *string
}

// Reconstruct rebuilds an entity from persistence data (no validation).
func Reconstruct(p ReconstructParams) *Entity {
	return &Entity{
		id: p.ID, productTypeID: p.ProductTypeID, productTypeCode: p.ProductTypeCode,
		productTypeName: p.ProductTypeName, grade: p.Grade, mode: p.Mode, value: p.Value,
		description: p.Description, oracleSysID: p.OracleSysID,
		createdAt: p.CreatedAt, createdBy: p.CreatedBy, updatedAt: p.UpdatedAt, updatedBy: p.UpdatedBy,
		deletedAt: p.DeletedAt, deletedBy: p.DeletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// ProductTypeID returns the cost_product_type.cpt_type_id.
func (e *Entity) ProductTypeID() int32 { return e.productTypeID }

// ProductTypeCode returns the joined product type code ("" on a fresh entity).
func (e *Entity) ProductTypeCode() string { return e.productTypeCode }

// ProductTypeName returns the joined product type name ("" on a fresh entity).
func (e *Entity) ProductTypeName() string { return e.productTypeName }

// Grade returns the grade.
func (e *Entity) Grade() Grade { return e.grade }

// Mode returns the mode.
func (e *Entity) Mode() Mode { return e.mode }

// Value returns the rule value.
func (e *Entity) Value() float64 { return e.value }

// Description returns the optional description.
func (e *Entity) Description() string { return e.description }

// OracleSysID returns the legacy CYTW_SYS_ID ("" when not migrated).
func (e *Entity) OracleSysID() string { return e.oracleSysID }

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

// DeletedBy returns who soft-deleted the rule.
func (e *Entity) DeletedBy() *string { return e.deletedBy }

// IsDeleted reports whether the rule is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// Apply computes this rule's grade weight for axWt.
func (e *Entity) Apply(axWt float64) float64 { return e.mode.Apply(axWt, e.value) }

// UpdateInput carries optional field mutations. Product type and grade are
// immutable: they form the rule's natural key.
type UpdateInput struct {
	Mode        *Mode
	Value       *float64
	Description *string
}

// Update applies optional field changes.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if in.Mode != nil {
		if _, err := ParseMode(string(*in.Mode)); err != nil {
			return err
		}
	}
	if in.Value != nil {
		if err := validateValue(*in.Value); err != nil {
			return err
		}
	}
	if in.Description != nil {
		if err := validateDescription(*in.Description); err != nil {
			return err
		}
	}
	if in.Mode != nil {
		e.mode = *in.Mode
	}
	if in.Value != nil {
		e.value = *in.Value
	}
	if in.Description != nil {
		e.description = *in.Description
	}
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the rule as deleted.
func (e *Entity) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	now := time.Now()
	e.deletedAt = &now
	e.deletedBy = &deletedBy
	return nil
}

func validateValue(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ErrInvalidValue
	}
	return nil
}

func validateDescription(d string) error {
	if utf8.RuneCountInString(d) > maxDescriptionLen {
		return ErrDescriptionTooLong
	}
	return nil
}
