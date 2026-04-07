// Package parameter provides domain logic for Parameter management.
package parameter

import (
	"time"

	"github.com/google/uuid"
)

// Parameter is the aggregate root for Parameter domain.
type Parameter struct {
	id            uuid.UUID
	code          Code
	name          string
	shortName     string
	dataType      DataType
	paramCategory ParamCategory
	uomID         *uuid.UUID
	uomCode       string
	uomName       string
	defaultValue  *string
	minValue      *string
	maxValue      *string
	isActive      bool
	createdAt     time.Time
	createdBy     string
	updatedAt     *time.Time
	updatedBy     *string
	deletedAt     *time.Time
	deletedBy     *string
}

// NewParameter creates a new Parameter entity with validation.
func NewParameter(
	code Code,
	name string,
	shortName string,
	dataType DataType,
	paramCategory ParamCategory,
	uomID *uuid.UUID,
	defaultValue *string,
	minValue *string,
	maxValue *string,
	createdBy string,
) (*Parameter, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 200 {
		return nil, ErrNameTooLong
	}
	if len(shortName) > 50 {
		return nil, ErrShortNameTooLong
	}
	if !dataType.IsValid() {
		return nil, ErrInvalidDataType
	}
	if !paramCategory.IsValid() {
		return nil, ErrInvalidParamCategory
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Parameter{
		id:            uuid.New(),
		code:          code,
		name:          name,
		shortName:     shortName,
		dataType:      dataType,
		paramCategory: paramCategory,
		uomID:         uomID,
		defaultValue:  defaultValue,
		minValue:      minValue,
		maxValue:      maxValue,
		isActive:      true,
		createdAt:     time.Now(),
		createdBy:     createdBy,
	}, nil
}

// ReconstructParameter reconstructs a Parameter entity from persistence data.
func ReconstructParameter(
	id uuid.UUID,
	code Code,
	name string,
	shortName string,
	dataType DataType,
	paramCategory ParamCategory,
	uomID *uuid.UUID,
	uomCode string,
	uomName string,
	defaultValue *string,
	minValue *string,
	maxValue *string,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
	deletedAt *time.Time,
	deletedBy *string,
) *Parameter {
	return &Parameter{
		id:            id,
		code:          code,
		name:          name,
		shortName:     shortName,
		dataType:      dataType,
		paramCategory: paramCategory,
		uomID:         uomID,
		uomCode:       uomCode,
		uomName:       uomName,
		defaultValue:  defaultValue,
		minValue:      minValue,
		maxValue:      maxValue,
		isActive:      isActive,
		createdAt:     createdAt,
		createdBy:     createdBy,
		updatedAt:     updatedAt,
		updatedBy:     updatedBy,
		deletedAt:     deletedAt,
		deletedBy:     deletedBy,
	}
}

// =============================================================================
// Getters - Expose internal state read-only
// =============================================================================

// ID returns the unique identifier.
func (p *Parameter) ID() uuid.UUID { return p.id }

// Code returns the parameter code.
func (p *Parameter) Code() Code { return p.code }

// Name returns the display name.
func (p *Parameter) Name() string { return p.name }

// ShortName returns the short name.
func (p *Parameter) ShortName() string { return p.shortName }

// DataType returns the data type.
func (p *Parameter) DataType() DataType { return p.dataType }

// ParamCategory returns the parameter category.
func (p *Parameter) ParamCategory() ParamCategory { return p.paramCategory }

// UOMID returns the optional UOM reference ID.
func (p *Parameter) UOMID() *uuid.UUID { return p.uomID }

// UOMCode returns the resolved UOM code (from join).
func (p *Parameter) UOMCode() string { return p.uomCode }

// UOMName returns the resolved UOM name (from join).
func (p *Parameter) UOMName() string { return p.uomName }

// DefaultValue returns the default value.
func (p *Parameter) DefaultValue() *string { return p.defaultValue }

// MinValue returns the minimum value.
func (p *Parameter) MinValue() *string { return p.minValue }

// MaxValue returns the maximum value.
func (p *Parameter) MaxValue() *string { return p.maxValue }

// IsActive returns whether the parameter is active.
func (p *Parameter) IsActive() bool { return p.isActive }

// CreatedAt returns the creation timestamp.
func (p *Parameter) CreatedAt() time.Time { return p.createdAt }

// CreatedBy returns the creator.
func (p *Parameter) CreatedBy() string { return p.createdBy }

// UpdatedAt returns the last update timestamp.
func (p *Parameter) UpdatedAt() *time.Time { return p.updatedAt }

// UpdatedBy returns the last updater.
func (p *Parameter) UpdatedBy() *string { return p.updatedBy }

// DeletedAt returns the soft delete timestamp.
func (p *Parameter) DeletedAt() *time.Time { return p.deletedAt }

// DeletedBy returns who deleted the record.
func (p *Parameter) DeletedBy() *string { return p.deletedBy }

// IsDeleted returns true if the parameter is soft deleted.
func (p *Parameter) IsDeleted() bool { return p.deletedAt != nil }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates the Parameter with new values.
// Uses double pointers for nullable optional fields to distinguish "not set" from "set to nil".
func (p *Parameter) Update(
	name *string,
	shortName *string,
	dataType *DataType,
	paramCategory *ParamCategory,
	uomID **uuid.UUID,
	defaultValue **string,
	minValue **string,
	maxValue **string,
	isActive *bool,
	updatedBy string,
) error {
	if p.IsDeleted() {
		return ErrAlreadyDeleted
	}

	if err := p.updateName(name); err != nil {
		return err
	}
	if err := p.updateShortName(shortName); err != nil {
		return err
	}
	if err := p.updateDataType(dataType); err != nil {
		return err
	}
	if err := p.updateParamCategory(paramCategory); err != nil {
		return err
	}

	p.applyOptionalFields(uomID, defaultValue, minValue, maxValue, isActive)

	now := time.Now()
	p.updatedAt = &now
	p.updatedBy = &updatedBy

	return nil
}

func (p *Parameter) updateName(name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return ErrEmptyName
	}
	if len(*name) > 200 {
		return ErrNameTooLong
	}
	p.name = *name
	return nil
}

func (p *Parameter) updateShortName(shortName *string) error {
	if shortName == nil {
		return nil
	}
	if len(*shortName) > 50 {
		return ErrShortNameTooLong
	}
	p.shortName = *shortName
	return nil
}

func (p *Parameter) updateDataType(dataType *DataType) error {
	if dataType == nil {
		return nil
	}
	if !dataType.IsValid() {
		return ErrInvalidDataType
	}
	p.dataType = *dataType
	return nil
}

func (p *Parameter) updateParamCategory(paramCategory *ParamCategory) error {
	if paramCategory == nil {
		return nil
	}
	if !paramCategory.IsValid() {
		return ErrInvalidParamCategory
	}
	p.paramCategory = *paramCategory
	return nil
}

func (p *Parameter) applyOptionalFields(
	uomID **uuid.UUID,
	defaultValue **string,
	minValue **string,
	maxValue **string,
	isActive *bool,
) {
	if uomID != nil {
		p.uomID = *uomID
	}
	if defaultValue != nil {
		p.defaultValue = *defaultValue
	}
	if minValue != nil {
		p.minValue = *minValue
	}
	if maxValue != nil {
		p.maxValue = *maxValue
	}
	if isActive != nil {
		p.isActive = *isActive
	}
}

// SoftDelete marks the parameter as deleted.
func (p *Parameter) SoftDelete(deletedBy string) error {
	if p.IsDeleted() {
		return ErrAlreadyDeleted
	}

	now := time.Now()
	p.deletedAt = &now
	p.deletedBy = &deletedBy
	p.isActive = false

	return nil
}

// Activate sets the parameter as active.
func (p *Parameter) Activate(updatedBy string) error {
	if p.IsDeleted() {
		return ErrAlreadyDeleted
	}

	p.isActive = true
	now := time.Now()
	p.updatedAt = &now
	p.updatedBy = &updatedBy

	return nil
}

// Deactivate sets the parameter as inactive.
func (p *Parameter) Deactivate(updatedBy string) error {
	if p.IsDeleted() {
		return ErrAlreadyDeleted
	}

	p.isActive = false
	now := time.Now()
	p.updatedAt = &now
	p.updatedBy = &updatedBy

	return nil
}
