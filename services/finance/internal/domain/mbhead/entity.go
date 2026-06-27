// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the MB Head domain.
type Entity struct {
	id              uuid.UUID
	oracleSysID     *string
	mbCosting       string
	mgtName         *string
	denier          *float64
	filament        *int
	dozing          *float64
	mbhCheckStatus  *string
	mbhStatus       *string
	mbhLdrPrsn      *float64
	mbhFinalProduct *string
	mbhCode         *string
	isActive        bool
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
	deletedAt       *time.Time
	deletedBy       *string
}

// New creates a new MB Head entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(mbCosting string, oracleSysID, mgtName *string, denier *float64, filament *int, dozing *float64, mbhCheckStatus, mbhStatus *string, mbhLdrPrsn *float64, mbhFinalProduct, mbhCode *string, createdBy string) (*Entity, error) {
	if mbCosting == "" {
		return nil, ErrEmptyMBCosting
	}
	if len(mbCosting) > 100 {
		return nil, ErrMBCostingTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), oracleSysID: oracleSysID, mbCosting: mbCosting, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing,
		mbhCheckStatus: mbhCheckStatus, mbhStatus: mbhStatus, mbhLdrPrsn: mbhLdrPrsn,
		mbhFinalProduct: mbhFinalProduct, mbhCode: mbhCode,
		isActive: true, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds an MB Head from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, oracleSysID *string, mbCosting string, mgtName *string, denier *float64, filament *int, dozing *float64, mbhCheckStatus, mbhStatus *string, mbhLdrPrsn *float64, mbhFinalProduct, mbhCode *string, isActive bool, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, oracleSysID: oracleSysID, mbCosting: mbCosting, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing,
		mbhCheckStatus: mbhCheckStatus, mbhStatus: mbhStatus, mbhLdrPrsn: mbhLdrPrsn,
		mbhFinalProduct: mbhFinalProduct, mbhCode: mbhCode,
		isActive:  isActive,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// OracleSysID returns the optional Oracle system ID for import reconciliation.
func (e *Entity) OracleSysID() *string { return e.oracleSysID }

// MBCosting returns the batch cost code identifier.
func (e *Entity) MBCosting() string { return e.mbCosting }

// MgtName returns the optional management display name.
func (e *Entity) MgtName() *string { return e.mgtName }

// Denier returns the optional yarn denier value.
func (e *Entity) Denier() *float64 { return e.denier }

// Filament returns the optional number of filaments.
func (e *Entity) Filament() *int { return e.filament }

// Dozing returns the optional dozing percentage.
func (e *Entity) Dozing() *float64 { return e.dozing }

// MBHCheckStatus returns the optional Oracle check status.
func (e *Entity) MBHCheckStatus() *string { return e.mbhCheckStatus }

// MBHStatus returns the optional Oracle head status.
func (e *Entity) MBHStatus() *string { return e.mbhStatus }

// MBHLdrPrsn returns the optional Oracle leader person value.
func (e *Entity) MBHLdrPrsn() *float64 { return e.mbhLdrPrsn }

// MBHFinalProduct returns the optional Oracle final product code.
func (e *Entity) MBHFinalProduct() *string { return e.mbhFinalProduct }

// MBHCode returns the optional Oracle MB head code.
func (e *Entity) MBHCode() *string { return e.mbhCode }

// IsActive returns whether the MB head is active.
func (e *Entity) IsActive() bool { return e.isActive }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() time.Time { return e.createdAt }

// CreatedBy returns the creator.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() *time.Time { return e.updatedAt }

// UpdatedBy returns the last updater.
func (e *Entity) UpdatedBy() *string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() *time.Time { return e.deletedAt }

// DeletedBy returns who soft-deleted the record.
func (e *Entity) DeletedBy() *string { return e.deletedBy }

// IsDeleted returns true if the MB head has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	MBCosting       *string
	MgtName         *string
	Denier          *float64
	Filament        *int
	Dozing          *float64
	MBHCheckStatus  *string
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHFinalProduct *string
	MBHCode         *string
	IsActive        *bool
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyMBCosting(in.MBCosting); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the MB head as deleted.
func (e *Entity) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	now := time.Now()
	e.deletedAt = &now
	e.deletedBy = &deletedBy
	e.isActive = false
	return nil
}

func (e *Entity) applyMBCosting(mbCosting *string) error {
	if mbCosting == nil {
		return nil
	}
	if *mbCosting == "" {
		return ErrEmptyMBCosting
	}
	if len(*mbCosting) > 100 {
		return ErrMBCostingTooLong
	}
	e.mbCosting = *mbCosting
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.MgtName != nil {
		e.mgtName = in.MgtName
	}
	if in.Denier != nil {
		e.denier = in.Denier
	}
	if in.Filament != nil {
		e.filament = in.Filament
	}
	if in.Dozing != nil {
		e.dozing = in.Dozing
	}
	if in.MBHCheckStatus != nil {
		e.mbhCheckStatus = in.MBHCheckStatus
	}
	if in.MBHStatus != nil {
		e.mbhStatus = in.MBHStatus
	}
	if in.MBHLdrPrsn != nil {
		e.mbhLdrPrsn = in.MBHLdrPrsn
	}
	if in.MBHFinalProduct != nil {
		e.mbhFinalProduct = in.MBHFinalProduct
	}
	if in.MBHCode != nil {
		e.mbhCode = in.MBHCode
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
}
