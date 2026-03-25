// Package rmcategory provides domain logic for Raw Material Category management.
package rmcategory

import (
	"time"

	"github.com/google/uuid"
)

// RMCategory is the aggregate root for Raw Material Category domain.
type RMCategory struct {
	id          uuid.UUID
	code        Code
	name        string
	description string
	isActive    bool
	createdAt   time.Time
	createdBy   string
	updatedAt   *time.Time
	updatedBy   *string
	deletedAt   *time.Time
	deletedBy   *string
}

// NewRMCategory creates a new RMCategory entity with validation.
func NewRMCategory(code Code, name string, description string, createdBy string) (*RMCategory, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 100 {
		return nil, ErrNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &RMCategory{
		id:          uuid.New(),
		code:        code,
		name:        name,
		description: description,
		isActive:    true,
		createdAt:   time.Now(),
		createdBy:   createdBy,
	}, nil
}

// ReconstructRMCategory reconstructs an RMCategory entity from persistence data.
// This is used by repository implementations to rebuild the entity from database.
func ReconstructRMCategory(
	id uuid.UUID,
	code Code,
	name string,
	description string,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
	deletedAt *time.Time,
	deletedBy *string,
) *RMCategory {
	return &RMCategory{
		id:          id,
		code:        code,
		name:        name,
		description: description,
		isActive:    isActive,
		createdAt:   createdAt,
		createdBy:   createdBy,
		updatedAt:   updatedAt,
		updatedBy:   updatedBy,
		deletedAt:   deletedAt,
		deletedBy:   deletedBy,
	}
}

// =============================================================================
// Getters - Expose internal state read-only
// =============================================================================

// ID returns the unique identifier.
func (r *RMCategory) ID() uuid.UUID { return r.id }

// Code returns the category code.
func (r *RMCategory) Code() Code { return r.code }

// Name returns the display name.
func (r *RMCategory) Name() string { return r.name }

// Description returns the description.
func (r *RMCategory) Description() string { return r.description }

// IsActive returns whether the category is active.
func (r *RMCategory) IsActive() bool { return r.isActive }

// CreatedAt returns the creation timestamp.
func (r *RMCategory) CreatedAt() time.Time { return r.createdAt }

// CreatedBy returns the creator.
func (r *RMCategory) CreatedBy() string { return r.createdBy }

// UpdatedAt returns the last update timestamp.
func (r *RMCategory) UpdatedAt() *time.Time { return r.updatedAt }

// UpdatedBy returns the last updater.
func (r *RMCategory) UpdatedBy() *string { return r.updatedBy }

// DeletedAt returns the soft delete timestamp.
func (r *RMCategory) DeletedAt() *time.Time { return r.deletedAt }

// DeletedBy returns who deleted the record.
func (r *RMCategory) DeletedBy() *string { return r.deletedBy }

// IsDeleted returns true if the category is soft deleted.
func (r *RMCategory) IsDeleted() bool { return r.deletedAt != nil }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates the RMCategory with new values.
func (r *RMCategory) Update(name *string, description *string, isActive *bool, updatedBy string) error {
	if r.IsDeleted() {
		return ErrAlreadyDeleted
	}

	if name != nil {
		if *name == "" {
			return ErrEmptyName
		}
		if len(*name) > 100 {
			return ErrNameTooLong
		}
		r.name = *name
	}

	if description != nil {
		r.description = *description
	}

	if isActive != nil {
		r.isActive = *isActive
	}

	now := time.Now()
	r.updatedAt = &now
	r.updatedBy = &updatedBy

	return nil
}

// SoftDelete marks the RMCategory as deleted.
func (r *RMCategory) SoftDelete(deletedBy string) error {
	if r.IsDeleted() {
		return ErrAlreadyDeleted
	}

	now := time.Now()
	r.deletedAt = &now
	r.deletedBy = &deletedBy
	r.isActive = false

	return nil
}

// Activate sets the RMCategory as active.
func (r *RMCategory) Activate(updatedBy string) error {
	if r.IsDeleted() {
		return ErrAlreadyDeleted
	}

	r.isActive = true
	now := time.Now()
	r.updatedAt = &now
	r.updatedBy = &updatedBy

	return nil
}

// Deactivate sets the RMCategory as inactive.
func (r *RMCategory) Deactivate(updatedBy string) error {
	if r.IsDeleted() {
		return ErrAlreadyDeleted
	}

	r.isActive = false
	now := time.Now()
	r.updatedAt = &now
	r.updatedBy = &updatedBy

	return nil
}
