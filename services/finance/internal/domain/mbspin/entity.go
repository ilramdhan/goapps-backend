// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the MB Spin domain.
type Entity struct {
	id             uuid.UUID
	oracleSysID    *string
	orionItemCode  *string
	headID         uuid.UUID
	mgtName        string
	denier         *float64
	filament       *int
	dozing         *float64
	mbCosting      *string
	cc             *string
	costRateMkt    *float64
	isActive       bool
	createdAt      time.Time
	createdBy      string
	updatedAt      *time.Time
	updatedBy      *string
	deletedAt      *time.Time
	deletedBy      *string
}

// New creates a new MB Spin entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(headID uuid.UUID, mgtName string, oracleSysID, orionItemCode *string, denier *float64, filament *int, dozing *float64, mbCosting *string, cc *string, costRateMkt *float64, createdBy string) (*Entity, error) {
	if headID == uuid.Nil {
		return nil, ErrInvalidHeadID
	}
	if mgtName == "" {
		return nil, ErrEmptyMgtName
	}
	if len(mgtName) > 100 {
		return nil, ErrMgtNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), oracleSysID: oracleSysID, orionItemCode: orionItemCode, headID: headID, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing, mbCosting: mbCosting,
		cc: cc, costRateMkt: costRateMkt,
		isActive: true, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds an MB Spin from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, oracleSysID, orionItemCode *string, headID uuid.UUID, mgtName string, denier *float64, filament *int, dozing *float64, mbCosting *string, cc *string, costRateMkt *float64, isActive bool, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, oracleSysID: oracleSysID, orionItemCode: orionItemCode, headID: headID, mgtName: mgtName,
		denier: denier, filament: filament, dozing: dozing, mbCosting: mbCosting,
		cc: cc, costRateMkt: costRateMkt,
		isActive: isActive, createdAt: createdAt, createdBy: createdBy,
		updatedAt: updatedAt, updatedBy: updatedBy, deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// OracleSysID returns the optional Oracle system ID.
func (e *Entity) OracleSysID() *string { return e.oracleSysID }

// OrionItemCode returns the optional Oracle ORION ERP item code (CMBS_ORION_ITEM_CODE).
func (e *Entity) OrionItemCode() *string { return e.orionItemCode }

// HeadID returns the parent MB head UUID.
func (e *Entity) HeadID() uuid.UUID { return e.headID }

// MgtName returns the management display name.
func (e *Entity) MgtName() string { return e.mgtName }

// Denier returns the optional spin denier.
func (e *Entity) Denier() *float64 { return e.denier }

// Filament returns the optional number of filaments.
func (e *Entity) Filament() *int { return e.filament }

// Dozing returns the optional dozing percentage.
func (e *Entity) Dozing() *float64 { return e.dozing }

// MBCosting returns the optional spin cost code.
func (e *Entity) MBCosting() *string { return e.mbCosting }

// CC returns the optional MB/SP cost code.
func (e *Entity) CC() *string { return e.cc }

// CostRateMkt returns the optional MB rate MKT USD/kg.
func (e *Entity) CostRateMkt() *float64 { return e.costRateMkt }

// IsActive returns whether the spin is active.
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

// IsDeleted returns true if the spin is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	MgtName     *string
	Denier      *float64
	Filament    *int
	Dozing      *float64
	MBCosting   *string
	CC          *string
	CostRateMkt *float64
	IsActive    *bool
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyMgtName(in.MgtName); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the spin as deleted.
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

func (e *Entity) applyMgtName(mgtName *string) error {
	if mgtName == nil {
		return nil
	}
	if *mgtName == "" {
		return ErrEmptyMgtName
	}
	if len(*mgtName) > 100 {
		return ErrMgtNameTooLong
	}
	e.mgtName = *mgtName
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.Denier != nil {
		e.denier = in.Denier
	}
	if in.Filament != nil {
		e.filament = in.Filament
	}
	if in.Dozing != nil {
		e.dozing = in.Dozing
	}
	if in.MBCosting != nil {
		e.mbCosting = in.MBCosting
	}
	if in.CC != nil {
		e.cc = in.CC
	}
	if in.CostRateMkt != nil {
		e.costRateMkt = in.CostRateMkt
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
}
