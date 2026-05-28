// Package group contains the BI DashboardGroup aggregate (left-rail grouping).
package group

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var codeRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Group is the BI dashboard group aggregate.
type Group struct {
	id           uuid.UUID
	code         string
	name         string
	description  string
	icon         string
	displayOrder int
	isActive     bool
	createdAt    time.Time
	createdBy    uuid.UUID
	updatedAt    time.Time
	updatedBy    uuid.UUID
}

// NewGroupParams holds validated inputs for NewGroup.
type NewGroupParams struct {
	ID           uuid.UUID
	Code         string
	Name         string
	Description  string
	Icon         string
	DisplayOrder int
	IsActive     bool
	CreatedBy    uuid.UUID
}

// NewGroup validates input and constructs a Group.
func NewGroup(p NewGroupParams) (*Group, error) {
	code := strings.TrimSpace(p.Code)
	if len(code) < 2 || len(code) > 40 || !codeRegex.MatchString(code) {
		return nil, fmt.Errorf("%w: group_code must match ^[A-Z][A-Z0-9_]*$ (2..40 chars)", ErrInvalidCode)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" || len(name) > 120 {
		return nil, fmt.Errorf("%w: group_name must be 1..120 chars", ErrInvalidName)
	}
	if len(p.Description) > 500 {
		return nil, fmt.Errorf("%w: description must be <= 500 chars", ErrInvalidName)
	}
	if len(p.Icon) > 40 {
		return nil, fmt.Errorf("%w: icon must be <= 40 chars", ErrInvalidName)
	}
	id := p.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &Group{
		id:           id,
		code:         code,
		name:         name,
		description:  p.Description,
		icon:         p.Icon,
		displayOrder: p.DisplayOrder,
		isActive:     p.IsActive,
		createdAt:    time.Now().UTC(),
		createdBy:    p.CreatedBy,
	}, nil
}

// UpdateParams holds optional fields for Update; nil pointer leaves untouched.
type UpdateParams struct {
	Name         *string
	Description  *string
	Icon         *string
	DisplayOrder *int
	IsActive     *bool
	UpdatedBy    uuid.UUID
}

// Update applies optional changes with validation.
func (g *Group) Update(p UpdateParams) error {
	staged := *g
	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 120 {
			return fmt.Errorf("%w: name 1..120 required", ErrInvalidName)
		}
		staged.name = n
	}
	if p.Description != nil {
		if len(*p.Description) > 500 {
			return fmt.Errorf("%w: description <= 500", ErrInvalidName)
		}
		staged.description = *p.Description
	}
	if p.Icon != nil {
		if len(*p.Icon) > 40 {
			return fmt.Errorf("%w: icon <= 40", ErrInvalidName)
		}
		staged.icon = *p.Icon
	}
	if p.DisplayOrder != nil {
		staged.displayOrder = *p.DisplayOrder
	}
	if p.IsActive != nil {
		staged.isActive = *p.IsActive
	}
	staged.updatedAt = time.Now().UTC()
	staged.updatedBy = p.UpdatedBy
	*g = staged
	return nil
}

// SetAuditFromHydration restores audit fields when loading from the database.
func (g *Group) SetAuditFromHydration(createdAt, updatedAt time.Time, createdBy, updatedBy uuid.UUID) {
	g.createdAt = createdAt
	g.updatedAt = updatedAt
	g.createdBy = createdBy
	g.updatedBy = updatedBy
}

// ID returns the group's unique identifier.
func (g *Group) ID() uuid.UUID { return g.id }

// Code returns the group's business code.
func (g *Group) Code() string { return g.code }

// Name returns the display name.
func (g *Group) Name() string { return g.name }

// Description returns the optional description.
func (g *Group) Description() string { return g.description }

// Icon returns the optional icon identifier.
func (g *Group) Icon() string { return g.icon }

// DisplayOrder returns the display ordering integer.
func (g *Group) DisplayOrder() int { return g.displayOrder }

// IsActive reports whether the group is active.
func (g *Group) IsActive() bool { return g.isActive }

// CreatedAt returns the creation timestamp.
func (g *Group) CreatedAt() time.Time { return g.createdAt }

// CreatedBy returns the UUID of the creating user.
func (g *Group) CreatedBy() uuid.UUID { return g.createdBy }

// UpdatedAt returns the last-update timestamp.
func (g *Group) UpdatedAt() time.Time { return g.updatedAt }

// UpdatedBy returns the UUID of the last updating user.
func (g *Group) UpdatedBy() uuid.UUID { return g.updatedBy }
