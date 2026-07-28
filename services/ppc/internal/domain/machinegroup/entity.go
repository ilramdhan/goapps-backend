// Package machinegroup provides domain logic for PPC machine-group master data.
package machinegroup

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
)

const maxNameLen = 50

// MachineGroup is the aggregate root for the machine-group master.
type MachineGroup struct {
	id        int64
	name      string
	groupArea area.Area
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
}

// NewMachineGroup creates a new MachineGroup with validation.
func NewMachineGroup(name string, groupArea area.Area, createdBy string) (*MachineGroup, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if groupArea.IsEmpty() {
		return nil, ErrInvalidArea
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &MachineGroup{
		name:      name,
		groupArea: groupArea,
		createdAt: time.Now(),
		createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds a MachineGroup from persistence (no validation).
func Reconstruct(
	id int64,
	name string,
	groupArea area.Area,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *MachineGroup {
	return &MachineGroup{
		id:        id,
		name:      name,
		groupArea: groupArea,
		createdAt: createdAt,
		createdBy: createdBy,
		updatedAt: updatedAt,
		updatedBy: updatedBy,
	}
}

// ID returns the group identifier.
func (g *MachineGroup) ID() int64 { return g.id }

// Name returns the group name.
func (g *MachineGroup) Name() string { return g.name }

// Area returns the group area.
func (g *MachineGroup) Area() area.Area { return g.groupArea }

// CreatedAt returns the creation timestamp.
func (g *MachineGroup) CreatedAt() time.Time { return g.createdAt }

// CreatedBy returns the creator.
func (g *MachineGroup) CreatedBy() string { return g.createdBy }

// UpdatedAt returns the last update timestamp.
func (g *MachineGroup) UpdatedAt() *time.Time { return g.updatedAt }

// UpdatedBy returns the last updater.
func (g *MachineGroup) UpdatedBy() *string { return g.updatedBy }

// Update applies optional field changes with validation.
func (g *MachineGroup) Update(name *string, groupArea *area.Area, updatedBy string) error {
	if name != nil {
		if err := validateName(*name); err != nil {
			return err
		}
		g.name = *name
	}
	if groupArea != nil {
		if groupArea.IsEmpty() {
			return ErrInvalidArea
		}
		g.groupArea = *groupArea
	}

	now := time.Now()
	g.updatedAt = &now
	g.updatedBy = &updatedBy
	return nil
}

func validateName(name string) error {
	if name == "" {
		return ErrEmptyName
	}
	if len(name) > maxNameLen {
		return ErrNameTooLong
	}
	return nil
}
