// Package user provides domain logic for User management.
package user

import (
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Detail contains extended profile information for a user.
type Detail struct {
	id             uuid.UUID
	userID         uuid.UUID
	sectionID      *uuid.UUID
	employeeCode   string
	fullName       string
	firstName      string
	lastName       string
	phone          string
	profilePicture string
	position       string
	dateOfBirth    *time.Time
	address        string
	extraData      map[string]interface{}
	audit          shared.AuditInfo
}

// NewDetail creates a new Detail entity.
func NewDetail(
	userID uuid.UUID,
	sectionID *uuid.UUID,
	employeeCode, fullName, firstName, lastName string,
	createdBy string,
) (*Detail, error) {
	if employeeCode == "" {
		return nil, shared.ErrEmptyCode
	}
	if fullName == "" {
		return nil, shared.ErrEmptyName
	}

	return &Detail{
		id:           uuid.New(),
		userID:       userID,
		sectionID:    sectionID,
		employeeCode: employeeCode,
		fullName:     fullName,
		firstName:    firstName,
		lastName:     lastName,
		audit:        shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructDetail reconstructs a Detail from persistence.
func ReconstructDetail(
	id, userID uuid.UUID,
	sectionID *uuid.UUID,
	employeeCode, fullName, firstName, lastName string,
	phone, profilePicture, position string,
	dateOfBirth *time.Time,
	address string,
	extraData map[string]interface{},
	audit shared.AuditInfo,
) *Detail {
	return &Detail{
		id:             id,
		userID:         userID,
		sectionID:      sectionID,
		employeeCode:   employeeCode,
		fullName:       fullName,
		firstName:      firstName,
		lastName:       lastName,
		phone:          phone,
		profilePicture: profilePicture,
		position:       position,
		dateOfBirth:    dateOfBirth,
		address:        address,
		extraData:      extraData,
		audit:          audit,
	}
}

// ID returns the detail identifier.
func (d *Detail) ID() uuid.UUID { return d.id }

// UserID returns the associated user identifier.
func (d *Detail) UserID() uuid.UUID { return d.userID }

// SectionID returns the section identifier.
func (d *Detail) SectionID() *uuid.UUID { return d.sectionID }

// EmployeeCode returns the employee code.
func (d *Detail) EmployeeCode() string { return d.employeeCode }

// FullName returns the full name.
func (d *Detail) FullName() string { return d.fullName }

// FirstName returns the first name.
func (d *Detail) FirstName() string { return d.firstName }

// LastName returns the last name.
func (d *Detail) LastName() string { return d.lastName }

// Phone returns the phone number.
func (d *Detail) Phone() string { return d.phone }

// ProfilePicture returns the profile picture URL.
func (d *Detail) ProfilePicture() string { return d.profilePicture }

// Position returns the position.
func (d *Detail) Position() string { return d.position }

// DateOfBirth returns the date of birth.
func (d *Detail) DateOfBirth() *time.Time { return d.dateOfBirth }

// Address returns the address.
func (d *Detail) Address() string { return d.address }

// ExtraData returns the extra data map.
func (d *Detail) ExtraData() map[string]interface{} { return d.extraData }

// Audit returns the audit information.
func (d *Detail) Audit() shared.AuditInfo { return d.audit }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates user detail fields.
func (d *Detail) Update(
	sectionID *uuid.UUID,
	fullName, firstName, lastName *string,
	phone, profilePicture, position *string,
	dateOfBirth *time.Time,
	address *string,
	extraData map[string]interface{},
	updatedBy string,
) error {
	if sectionID != nil {
		d.sectionID = sectionID
	}
	if fullName != nil {
		if *fullName == "" {
			return shared.ErrEmptyName
		}
		d.fullName = *fullName
	}
	if firstName != nil {
		d.firstName = *firstName
	}
	if lastName != nil {
		d.lastName = *lastName
	}
	if phone != nil {
		d.phone = *phone
	}
	if profilePicture != nil {
		d.profilePicture = *profilePicture
	}
	if position != nil {
		d.position = *position
	}
	if dateOfBirth != nil {
		d.dateOfBirth = dateOfBirth
	}
	if address != nil {
		d.address = *address
	}
	if extraData != nil {
		d.extraData = extraData
	}

	d.audit.Update(updatedBy)
	return nil
}
