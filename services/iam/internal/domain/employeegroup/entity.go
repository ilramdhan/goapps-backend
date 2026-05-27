// Package employeegroup provides domain logic for Employee Group management.
package employeegroup

import (
	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// EmployeeGroup is the aggregate root for Employee Group domain.
type EmployeeGroup struct {
	id       uuid.UUID
	code     Code
	name     string
	isActive bool
	audit    shared.AuditInfo
}

// NewEmployeeGroup creates a new EmployeeGroup with validation.
func NewEmployeeGroup(code Code, name, createdBy string) (*EmployeeGroup, error) {
	if code.IsEmpty() {
		return nil, shared.ErrEmptyCode
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}
	if len(name) > 100 {
		return nil, shared.ErrNameTooLong
	}
	return &EmployeeGroup{
		id:       uuid.New(),
		code:     code,
		name:     name,
		isActive: true,
		audit:    shared.NewAuditInfo(createdBy),
	}, nil
}

// Reconstruct rebuilds an EmployeeGroup from persistence (no validation).
func Reconstruct(id uuid.UUID, code Code, name string, isActive bool, audit shared.AuditInfo) *EmployeeGroup {
	return &EmployeeGroup{
		id:       id,
		code:     code,
		name:     name,
		isActive: isActive,
		audit:    audit,
	}
}

// ID returns the identifier.
func (e *EmployeeGroup) ID() uuid.UUID { return e.id }

// Code returns the code value object.
func (e *EmployeeGroup) Code() Code { return e.code }

// Name returns the display name.
func (e *EmployeeGroup) Name() string { return e.name }

// IsActive returns whether the record is active.
func (e *EmployeeGroup) IsActive() bool { return e.isActive }

// Audit returns the audit information.
func (e *EmployeeGroup) Audit() shared.AuditInfo { return e.audit }

// IsDeleted returns whether the record has been soft-deleted.
func (e *EmployeeGroup) IsDeleted() bool { return e.audit.IsDeleted() }

// Update updates mutable fields. Each pointer is optional (nil means "no change").
func (e *EmployeeGroup) Update(name *string, isActive *bool, updatedBy string) error {
	if e.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if err := e.applyName(name); err != nil {
		return err
	}
	if isActive != nil {
		e.isActive = *isActive
	}
	e.audit.Update(updatedBy)
	return nil
}

func (e *EmployeeGroup) applyName(name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return shared.ErrEmptyName
	}
	if len(*name) > 100 {
		return shared.ErrNameTooLong
	}
	e.name = *name
	return nil
}

// SoftDelete marks the record as deleted.
func (e *EmployeeGroup) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	e.isActive = false
	e.audit.SoftDelete(deletedBy)
	return nil
}
