// Package companymapping provides domain logic for Company Mapping management.
package companymapping

import (
	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Hierarchy bundles the resolved hierarchy snapshot for a company mapping.
// Section is optional. The denormalized code/name fields are populated by
// the repository when reading from the database.
type Hierarchy struct {
	CompanyID      uuid.UUID
	CompanyCode    string
	CompanyName    string
	DivisionID     uuid.UUID
	DivisionCode   string
	DivisionName   string
	DepartmentID   uuid.UUID
	DepartmentCode string
	DepartmentName string
	SectionID      *uuid.UUID
	SectionCode    string
	SectionName    string
}

// CompanyMapping is the aggregate root for the Company Mapping domain.
type CompanyMapping struct {
	id        uuid.UUID
	code      Code
	name      Name
	hierarchy Hierarchy
	isActive  bool
	audit     shared.AuditInfo
}

// NewCompanyMapping creates a new CompanyMapping with validation.
// The denormalized hierarchy code/name fields are not validated here — the
// repository fills them on read.
func NewCompanyMapping(code Code, name Name, h Hierarchy, createdBy string) (*CompanyMapping, error) {
	if code.IsEmpty() {
		return nil, shared.ErrEmptyCode
	}
	if name.IsEmpty() {
		return nil, shared.ErrEmptyName
	}
	if h.CompanyID == uuid.Nil {
		return nil, ErrInvalidCompanyID
	}
	if h.DivisionID == uuid.Nil {
		return nil, ErrInvalidDivisionID
	}
	if h.DepartmentID == uuid.Nil {
		return nil, ErrInvalidDepartmentID
	}
	return &CompanyMapping{
		id:        uuid.New(),
		code:      code,
		name:      name,
		hierarchy: h,
		isActive:  true,
		audit:     shared.NewAuditInfo(createdBy),
	}, nil
}

// Reconstruct rebuilds an entity from persistence (no validation).
func Reconstruct(id uuid.UUID, code Code, name Name, h Hierarchy, isActive bool, audit shared.AuditInfo) *CompanyMapping {
	return &CompanyMapping{
		id:        id,
		code:      code,
		name:      name,
		hierarchy: h,
		isActive:  isActive,
		audit:     audit,
	}
}

// ID returns the identifier.
func (c *CompanyMapping) ID() uuid.UUID { return c.id }

// Code returns the code value object.
func (c *CompanyMapping) Code() Code { return c.code }

// Name returns the name value object.
func (c *CompanyMapping) Name() Name { return c.name }

// Hierarchy returns the resolved hierarchy snapshot.
func (c *CompanyMapping) Hierarchy() Hierarchy { return c.hierarchy }

// IsActive returns whether the record is active.
func (c *CompanyMapping) IsActive() bool { return c.isActive }

// Audit returns the audit information.
func (c *CompanyMapping) Audit() shared.AuditInfo { return c.audit }

// IsDeleted returns whether the record has been soft-deleted.
func (c *CompanyMapping) IsDeleted() bool { return c.audit.IsDeleted() }

// Update updates mutable fields. All pointers optional (nil means "no change").
// Code is immutable. Hierarchy fields are passed individually to allow partial
// updates; the repository fills code/name fields on the next read.
func (c *CompanyMapping) Update(
	name *Name,
	companyID, divisionID, departmentID *uuid.UUID,
	sectionID *uuid.UUID,
	clearSection bool,
	isActive *bool,
	updatedBy string,
) error {
	if c.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if err := c.applyName(name); err != nil {
		return err
	}
	if err := c.applyHierarchy(companyID, divisionID, departmentID); err != nil {
		return err
	}
	c.applySection(sectionID, clearSection)
	if isActive != nil {
		c.isActive = *isActive
	}
	c.audit.Update(updatedBy)
	return nil
}

func (c *CompanyMapping) applyName(name *Name) error {
	if name == nil {
		return nil
	}
	if name.IsEmpty() {
		return shared.ErrEmptyName
	}
	c.name = *name
	return nil
}

func (c *CompanyMapping) applyHierarchy(companyID, divisionID, departmentID *uuid.UUID) error {
	if companyID != nil {
		if *companyID == uuid.Nil {
			return ErrInvalidCompanyID
		}
		c.hierarchy.CompanyID = *companyID
	}
	if divisionID != nil {
		if *divisionID == uuid.Nil {
			return ErrInvalidDivisionID
		}
		c.hierarchy.DivisionID = *divisionID
	}
	if departmentID != nil {
		if *departmentID == uuid.Nil {
			return ErrInvalidDepartmentID
		}
		c.hierarchy.DepartmentID = *departmentID
	}
	return nil
}

func (c *CompanyMapping) applySection(sectionID *uuid.UUID, clearSection bool) {
	if clearSection {
		c.hierarchy.SectionID = nil
		c.hierarchy.SectionCode = ""
		c.hierarchy.SectionName = ""
		return
	}
	if sectionID != nil {
		id := *sectionID
		c.hierarchy.SectionID = &id
	}
}

// SoftDelete marks the record as deleted.
func (c *CompanyMapping) SoftDelete(deletedBy string) error {
	if c.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	c.isActive = false
	c.audit.SoftDelete(deletedBy)
	return nil
}
