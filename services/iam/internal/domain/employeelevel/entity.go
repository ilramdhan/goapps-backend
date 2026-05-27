// Package employeelevel provides domain logic for Employee Level management.
package employeelevel

import (
	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// EmployeeLevel is the aggregate root for Employee Level domain.
type EmployeeLevel struct {
	id       uuid.UUID
	code     Code
	name     string
	grade    int32
	typ      Type
	sequence int32
	workflow Workflow
	isActive bool
	audit    shared.AuditInfo
}

// NewEmployeeLevel creates a new EmployeeLevel with validation.
func NewEmployeeLevel(code Code, name string, grade int32, typ Type, sequence int32, workflow Workflow, createdBy string) (*EmployeeLevel, error) {
	if code.IsEmpty() {
		return nil, shared.ErrEmptyCode
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}
	if len(name) > 100 {
		return nil, shared.ErrNameTooLong
	}
	if grade < 0 || grade > maxGrade {
		return nil, ErrInvalidGrade
	}
	if sequence < 0 || sequence > maxSeq {
		return nil, ErrInvalidSequence
	}
	if !typ.IsValid() {
		return nil, ErrInvalidType
	}
	if !workflow.IsValid() {
		return nil, ErrInvalidWorkflow
	}

	return &EmployeeLevel{
		id:       uuid.New(),
		code:     code,
		name:     name,
		grade:    grade,
		typ:      typ,
		sequence: sequence,
		workflow: workflow,
		isActive: true,
		audit:    shared.NewAuditInfo(createdBy),
	}, nil
}

// Reconstruct rebuilds an EmployeeLevel from persistence (no validation).
func Reconstruct(id uuid.UUID, code Code, name string, grade int32, typ Type, sequence int32, workflow Workflow, isActive bool, audit shared.AuditInfo) *EmployeeLevel {
	return &EmployeeLevel{
		id:       id,
		code:     code,
		name:     name,
		grade:    grade,
		typ:      typ,
		sequence: sequence,
		workflow: workflow,
		isActive: isActive,
		audit:    audit,
	}
}

// ID returns the identifier.
func (e *EmployeeLevel) ID() uuid.UUID { return e.id }

// Code returns the code value object.
func (e *EmployeeLevel) Code() Code { return e.code }

// Name returns the display name.
func (e *EmployeeLevel) Name() string { return e.name }

// Grade returns the numeric grade.
func (e *EmployeeLevel) Grade() int32 { return e.grade }

// Type returns the functional type.
func (e *EmployeeLevel) Type() Type { return e.typ }

// Sequence returns the sort sequence.
func (e *EmployeeLevel) Sequence() int32 { return e.sequence }

// Workflow returns the workflow state.
func (e *EmployeeLevel) Workflow() Workflow { return e.workflow }

// IsActive returns whether the record is active.
func (e *EmployeeLevel) IsActive() bool { return e.isActive }

// Audit returns the audit information.
func (e *EmployeeLevel) Audit() shared.AuditInfo { return e.audit }

// IsDeleted returns whether the record has been soft-deleted.
func (e *EmployeeLevel) IsDeleted() bool { return e.audit.IsDeleted() }

// Update updates mutable fields. Each pointer is optional (nil means "no change").
func (e *EmployeeLevel) Update(name *string, grade *int32, typ *Type, sequence *int32, workflow *Workflow, isActive *bool, updatedBy string) error {
	if e.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if err := e.applyName(name); err != nil {
		return err
	}
	if err := e.applyGrade(grade); err != nil {
		return err
	}
	if err := e.applyType(typ); err != nil {
		return err
	}
	if err := e.applySequence(sequence); err != nil {
		return err
	}
	if err := e.applyWorkflow(workflow); err != nil {
		return err
	}
	if isActive != nil {
		e.isActive = *isActive
	}
	e.audit.Update(updatedBy)
	return nil
}

func (e *EmployeeLevel) applyName(name *string) error {
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

func (e *EmployeeLevel) applyGrade(grade *int32) error {
	if grade == nil {
		return nil
	}
	if *grade < 0 || *grade > maxGrade {
		return ErrInvalidGrade
	}
	e.grade = *grade
	return nil
}

func (e *EmployeeLevel) applyType(typ *Type) error {
	if typ == nil {
		return nil
	}
	if !typ.IsValid() {
		return ErrInvalidType
	}
	e.typ = *typ
	return nil
}

func (e *EmployeeLevel) applySequence(sequence *int32) error {
	if sequence == nil {
		return nil
	}
	if *sequence < 0 || *sequence > maxSeq {
		return ErrInvalidSequence
	}
	e.sequence = *sequence
	return nil
}

func (e *EmployeeLevel) applyWorkflow(workflow *Workflow) error {
	if workflow == nil {
		return nil
	}
	if !workflow.IsValid() {
		return ErrInvalidWorkflow
	}
	e.workflow = *workflow
	return nil
}

// SetWorkflow updates the workflow state directly (used by workflow handlers after validation).
func (e *EmployeeLevel) SetWorkflow(wf Workflow, updatedBy string) {
	e.workflow = wf
	e.audit.Update(updatedBy)
}

// SoftDelete marks the record as deleted.
func (e *EmployeeLevel) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	e.isActive = false
	e.audit.SoftDelete(deletedBy)
	return nil
}
