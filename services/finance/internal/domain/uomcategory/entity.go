// Package uomcategory provides domain logic for UOM Category management.
package uomcategory

import (
	"time"

	"github.com/google/uuid"
)

// Category is the aggregate root for UOM Category domain.
type Category struct {
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

// NewCategory creates a new Category entity with validation.
func NewCategory(code Code, name string, description string, createdBy string) (*Category, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 100 {
		return nil, ErrNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Category{
		id:          uuid.New(),
		code:        code,
		name:        name,
		description: description,
		isActive:    true,
		createdAt:   time.Now(),
		createdBy:   createdBy,
	}, nil
}

// ReconstructCategory reconstructs a Category entity from persistence data.
func ReconstructCategory(
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
) *Category {
	return &Category{
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
func (c *Category) ID() uuid.UUID { return c.id }

// Code returns the category code.
func (c *Category) Code() Code { return c.code }

// Name returns the display name.
func (c *Category) Name() string { return c.name }

// Description returns the description.
func (c *Category) Description() string { return c.description }

// IsActive returns whether the category is active.
func (c *Category) IsActive() bool { return c.isActive }

// CreatedAt returns the creation timestamp.
func (c *Category) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *Category) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *Category) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *Category) UpdatedBy() *string { return c.updatedBy }

// DeletedAt returns the soft delete timestamp.
func (c *Category) DeletedAt() *time.Time { return c.deletedAt }

// DeletedBy returns who deleted the record.
func (c *Category) DeletedBy() *string { return c.deletedBy }

// IsDeleted returns true if the category is soft deleted.
func (c *Category) IsDeleted() bool { return c.deletedAt != nil }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates the Category with new values.
func (c *Category) Update(name *string, description *string, isActive *bool, updatedBy string) error {
	if c.IsDeleted() {
		return ErrAlreadyDeleted
	}

	if name != nil {
		if *name == "" {
			return ErrEmptyName
		}
		if len(*name) > 100 {
			return ErrNameTooLong
		}
		c.name = *name
	}

	if description != nil {
		c.description = *description
	}

	if isActive != nil {
		c.isActive = *isActive
	}

	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy

	return nil
}

// SoftDelete marks the Category as deleted.
func (c *Category) SoftDelete(deletedBy string) error {
	if c.IsDeleted() {
		return ErrAlreadyDeleted
	}

	now := time.Now()
	c.deletedAt = &now
	c.deletedBy = &deletedBy
	c.isActive = false

	return nil
}

// Activate sets the Category as active.
func (c *Category) Activate(updatedBy string) error {
	if c.IsDeleted() {
		return ErrAlreadyDeleted
	}

	c.isActive = true
	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy

	return nil
}

// Deactivate sets the Category as inactive.
func (c *Category) Deactivate(updatedBy string) error {
	if c.IsDeleted() {
		return ErrAlreadyDeleted
	}

	c.isActive = false
	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy

	return nil
}
