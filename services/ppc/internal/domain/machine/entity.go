// Package machine provides domain logic for PPC machine master data.
package machine

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
)

// Machine is the aggregate root for the machine master.
//
// Machine rows are sync-sourced from finance/Oracle; only PPC-local fields are
// mutable through this domain (area, line, group, doff weight, active, orion code).
type Machine struct {
	id           int64
	machineNo    string
	area         area.Area
	line         *string
	groupID      *int64
	groupName    string
	doffWeightKg *float64
	isActive     bool
	orionCode    string
	sourceMcID   *string
	syncedAt     *time.Time
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
}

// Reconstruct rebuilds a Machine from persistence (no validation).
func Reconstruct(
	id int64,
	machineNo string,
	machineArea area.Area,
	line *string,
	groupID *int64,
	groupName string,
	doffWeightKg *float64,
	isActive bool,
	orionCode string,
	sourceMcID *string,
	syncedAt *time.Time,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Machine {
	return &Machine{
		id:           id,
		machineNo:    machineNo,
		area:         machineArea,
		line:         line,
		groupID:      groupID,
		groupName:    groupName,
		doffWeightKg: doffWeightKg,
		isActive:     isActive,
		orionCode:    orionCode,
		sourceMcID:   sourceMcID,
		syncedAt:     syncedAt,
		createdAt:    createdAt,
		createdBy:    createdBy,
		updatedAt:    updatedAt,
		updatedBy:    updatedBy,
	}
}

// ID returns the machine identifier.
func (m *Machine) ID() int64 { return m.id }

// No returns the machine number.
func (m *Machine) No() string { return m.machineNo }

// Area returns the machine area.
func (m *Machine) Area() area.Area { return m.area }

// Line returns the optional machine line.
func (m *Machine) Line() *string { return m.line }

// GroupID returns the optional machine-group ID.
func (m *Machine) GroupID() *int64 { return m.groupID }

// GroupName returns the denormalized machine-group name.
func (m *Machine) GroupName() string { return m.groupName }

// DoffWeightKg returns the optional doff weight in kilograms.
func (m *Machine) DoffWeightKg() *float64 { return m.doffWeightKg }

// IsActive returns whether the machine is active.
func (m *Machine) IsActive() bool { return m.isActive }

// OrionCode returns the machine Orion code.
func (m *Machine) OrionCode() string { return m.orionCode }

// SourceMcID returns the optional finance source machine UUID.
func (m *Machine) SourceMcID() *string { return m.sourceMcID }

// SyncedAt returns the optional last-sync timestamp.
func (m *Machine) SyncedAt() *time.Time { return m.syncedAt }

// CreatedAt returns the creation timestamp.
func (m *Machine) CreatedAt() time.Time { return m.createdAt }

// CreatedBy returns the creator.
func (m *Machine) CreatedBy() string { return m.createdBy }

// UpdatedAt returns the last update timestamp.
func (m *Machine) UpdatedAt() *time.Time { return m.updatedAt }

// UpdatedBy returns the last updater.
func (m *Machine) UpdatedBy() *string { return m.updatedBy }

// Update applies optional PPC-local field changes with validation.
func (m *Machine) Update(
	machineArea *area.Area,
	line *string,
	groupID *int64,
	doffWeightKg *float64,
	isActive *bool,
	orionCode *string,
	updatedBy string,
) error {
	if updatedBy == "" {
		return ErrEmptyUpdatedBy
	}
	if machineArea != nil {
		if machineArea.IsEmpty() {
			return ErrInvalidArea
		}
		m.area = *machineArea
	}
	if line != nil {
		m.line = line
	}
	if groupID != nil {
		m.groupID = groupID
	}
	if doffWeightKg != nil {
		m.doffWeightKg = doffWeightKg
	}
	if isActive != nil {
		m.isActive = *isActive
	}
	if orionCode != nil {
		m.orionCode = *orionCode
	}

	now := time.Now()
	m.updatedAt = &now
	m.updatedBy = &updatedBy
	return nil
}
