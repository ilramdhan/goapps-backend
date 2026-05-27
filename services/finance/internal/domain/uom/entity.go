// Package uom provides domain logic for Unit of Measure management.
package uom

import (
	"time"

	"github.com/google/uuid"
)

// UOM is the aggregate root for Unit of Measure domain.
type UOM struct {
	id           uuid.UUID
	code         Code
	name         string
	categoryInfo CategoryInfo
	description  string
	isActive     bool
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
	deletedAt    *time.Time
	deletedBy    *string
}

// NewUOM creates a new UOM entity with validation.
func NewUOM(code Code, name string, categoryID uuid.UUID, description string, createdBy string) (*UOM, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 100 {
		return nil, ErrNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	if categoryID == uuid.Nil {
		return nil, ErrInvalidCategory
	}

	return &UOM{
		id:           uuid.New(),
		code:         code,
		name:         name,
		categoryInfo: CategoryInfo{id: categoryID},
		description:  description,
		isActive:     true,
		createdAt:    time.Now(),
		createdBy:    createdBy,
	}, nil
}

// ReconstructUOM reconstructs a UOM entity from persistence data.
// This is used by repository implementations to rebuild the entity from database.
func ReconstructUOM(
	id uuid.UUID,
	code Code,
	name string,
	categoryInfo CategoryInfo,
	description string,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
	deletedAt *time.Time,
	deletedBy *string,
) *UOM {
	return &UOM{
		id:           id,
		code:         code,
		name:         name,
		categoryInfo: categoryInfo,
		description:  description,
		isActive:     isActive,
		createdAt:    createdAt,
		createdBy:    createdBy,
		updatedAt:    updatedAt,
		updatedBy:    updatedBy,
		deletedAt:    deletedAt,
		deletedBy:    deletedBy,
	}
}

// =============================================================================
// Getters - Expose internal state read-only
// =============================================================================

// ID returns the unique identifier.
func (u *UOM) ID() uuid.UUID { return u.id }

// Code returns the UOM code.
func (u *UOM) Code() Code { return u.code }

// Name returns the display name.
func (u *UOM) Name() string { return u.name }

// CategoryInfo returns the category info.
func (u *UOM) CategoryInfo() CategoryInfo { return u.categoryInfo }

// CategoryID returns the category UUID.
func (u *UOM) CategoryID() uuid.UUID { return u.categoryInfo.id }

// Description returns the description.
func (u *UOM) Description() string { return u.description }

// IsActive returns whether the UOM is active.
func (u *UOM) IsActive() bool { return u.isActive }

// CreatedAt returns the creation timestamp.
func (u *UOM) CreatedAt() time.Time { return u.createdAt }

// CreatedBy returns the creator.
func (u *UOM) CreatedBy() string { return u.createdBy }

// UpdatedAt returns the last update timestamp.
func (u *UOM) UpdatedAt() *time.Time { return u.updatedAt }

// UpdatedBy returns the last updater.
func (u *UOM) UpdatedBy() *string { return u.updatedBy }

// DeletedAt returns the soft delete timestamp.
func (u *UOM) DeletedAt() *time.Time { return u.deletedAt }

// DeletedBy returns who deleted the record.
func (u *UOM) DeletedBy() *string { return u.deletedBy }

// IsDeleted returns true if the UOM is soft deleted.
func (u *UOM) IsDeleted() bool { return u.deletedAt != nil }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates the UOM with new values.
func (u *UOM) Update(name *string, categoryID *uuid.UUID, description *string, isActive *bool, updatedBy string) error {
	if u.IsDeleted() {
		return ErrAlreadyDeleted
	}

	if name != nil {
		if *name == "" {
			return ErrEmptyName
		}
		u.name = *name
	}

	if categoryID != nil {
		if *categoryID == uuid.Nil {
			return ErrInvalidCategory
		}
		u.categoryInfo = CategoryInfo{id: *categoryID}
	}

	if description != nil {
		u.description = *description
	}

	if isActive != nil {
		u.isActive = *isActive
	}

	now := time.Now()
	u.updatedAt = &now
	u.updatedBy = &updatedBy

	return nil
}

// SoftDelete marks the UOM as deleted.
func (u *UOM) SoftDelete(deletedBy string) error {
	if u.IsDeleted() {
		return ErrAlreadyDeleted
	}

	now := time.Now()
	u.deletedAt = &now
	u.deletedBy = &deletedBy
	u.isActive = false

	return nil
}

// Activate sets the UOM as active.
func (u *UOM) Activate(updatedBy string) error {
	if u.IsDeleted() {
		return ErrAlreadyDeleted
	}

	u.isActive = true
	now := time.Now()
	u.updatedAt = &now
	u.updatedBy = &updatedBy

	return nil
}

// Deactivate sets the UOM as inactive.
func (u *UOM) Deactivate(updatedBy string) error {
	if u.IsDeleted() {
		return ErrAlreadyDeleted
	}

	u.isActive = false
	now := time.Now()
	u.updatedAt = &now
	u.updatedBy = &updatedBy

	return nil
}
