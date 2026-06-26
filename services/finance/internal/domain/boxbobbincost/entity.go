// Package boxbobbincost provides domain logic for Box Bobbin Cost configuration management.
package boxbobbincost

import (
	"time"

	"github.com/google/uuid"
)

// Entity is the aggregate root for the Box Bobbin Cost domain (parent config).
type Entity struct {
	id           uuid.UUID
	code         string
	name         string
	bbcType      string
	noOfBob      int
	isActive     bool
	rates        []*RateEntry
	notes        string
	bbnReuse     *float64
	boxReuse     *float64
	boxCost      *float64
	bobinCost    *float64
	boxCostVal   *float64
	bobinCostVal *float64
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
	deletedAt    *time.Time
	deletedBy    *string
}

// RateEntry represents period-specific market and valuation rates.
type RateEntry struct {
	id         uuid.UUID
	parentID   uuid.UUID
	period     string
	bobRateMkt float64
	boxRateMkt float64
	bobRateVal *float64
	boxRateVal *float64
	createdAt  time.Time
	createdBy  string
	updatedAt  *time.Time
	updatedBy  *string
	deletedAt  *time.Time
	deletedBy  *string
}

// New creates a new Box Bobbin Cost entity with validation.
//
//nolint:revive // Many parameters required for construction.
func New(code, name, bbcType string, noOfBob int, notes string, bbnReuse, boxReuse, boxCost, bobinCost, boxCostVal, bobinCostVal *float64, createdBy string) (*Entity, error) {
	if code == "" {
		return nil, ErrEmptyCode
	}
	if len(code) > 30 {
		return nil, ErrCodeTooLong
	}
	if name == "" {
		return nil, ErrEmptyName
	}
	if len(name) > 100 {
		return nil, ErrNameTooLong
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), code: code, name: name, bbcType: bbcType,
		noOfBob: noOfBob, isActive: true, notes: notes,
		bbnReuse: bbnReuse, boxReuse: boxReuse, boxCost: boxCost,
		bobinCost: bobinCost, boxCostVal: boxCostVal, bobinCostVal: bobinCostVal,
		createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds an Entity from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id uuid.UUID, code, name, bbcType string, noOfBob int, isActive bool, rates []*RateEntry, notes string, bbnReuse, boxReuse, boxCost, bobinCost, boxCostVal, bobinCostVal *float64, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *Entity {
	return &Entity{
		id: id, code: code, name: name, bbcType: bbcType, noOfBob: noOfBob,
		isActive: isActive, rates: rates, notes: notes,
		bbnReuse: bbnReuse, boxReuse: boxReuse, boxCost: boxCost,
		bobinCost: bobinCost, boxCostVal: boxCostVal, bobinCostVal: bobinCostVal,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// NewRateEntry creates a new rate entry for a given period.
func NewRateEntry(parentID uuid.UUID, period string, bobRateMkt, boxRateMkt float64, bobRateVal, boxRateVal *float64, createdBy string) *RateEntry {
	return &RateEntry{
		id: uuid.New(), parentID: parentID, period: period,
		bobRateMkt: bobRateMkt, boxRateMkt: boxRateMkt,
		bobRateVal: bobRateVal, boxRateVal: boxRateVal,
		createdAt: time.Now(), createdBy: createdBy,
	}
}

// ReconstructRateEntry rebuilds a RateEntry from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func ReconstructRateEntry(id, parentID uuid.UUID, period string, bobRateMkt, boxRateMkt float64, bobRateVal, boxRateVal *float64, createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string, deletedAt *time.Time, deletedBy *string) *RateEntry {
	return &RateEntry{
		id: id, parentID: parentID, period: period,
		bobRateMkt: bobRateMkt, boxRateMkt: boxRateMkt,
		bobRateVal: bobRateVal, boxRateVal: boxRateVal,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// =============================================================================
// Entity Getters
// =============================================================================

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// Code returns the entity code.
func (e *Entity) Code() string { return e.code }

// Name returns the display name.
func (e *Entity) Name() string { return e.name }

// BBCType returns the type classification.
func (e *Entity) BBCType() string { return e.bbcType }

// NoOfBob returns the number of bobbins per unit.
func (e *Entity) NoOfBob() int { return e.noOfBob }

// IsActive returns whether the record is active.
func (e *Entity) IsActive() bool { return e.isActive }

// Rates returns the period rate entries.
func (e *Entity) Rates() []*RateEntry { return e.rates }

// Notes returns optional notes.
func (e *Entity) Notes() string { return e.notes }

// BbnReuse returns the optional bobbin reuse count.
func (e *Entity) BbnReuse() *float64 { return e.bbnReuse }

// BoxReuse returns the optional box reuse count.
func (e *Entity) BoxReuse() *float64 { return e.boxReuse }

// BoxCost returns the optional MKT box rate in USD/box.
func (e *Entity) BoxCost() *float64 { return e.boxCost }

// BobinCost returns the optional MKT bobbin rate in USD/bobbin.
func (e *Entity) BobinCost() *float64 { return e.bobinCost }

// BoxCostVal returns the optional VAL box rate in USD/box.
func (e *Entity) BoxCostVal() *float64 { return e.boxCostVal }

// BobinCostVal returns the optional VAL bobbin rate in USD/bobbin.
func (e *Entity) BobinCostVal() *float64 { return e.bobinCostVal }

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

// IsDeleted returns true if the entity is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// =============================================================================
// Entity Behavior
// =============================================================================

// UpdateInput carries optional field mutations for Update.
type UpdateInput struct {
	Name         *string
	BBCType      *string
	NoOfBob      *int
	Notes        *string
	IsActive     *bool
	BbnReuse     *float64
	BoxReuse     *float64
	BoxCost      *float64
	BobinCost    *float64
	BoxCostVal   *float64
	BobinCostVal *float64
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyName(in.Name); err != nil {
		return err
	}
	e.applyOptionalFields(in)
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the entity as deleted.
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

func (e *Entity) applyName(name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return ErrEmptyName
	}
	if len(*name) > 100 {
		return ErrNameTooLong
	}
	e.name = *name
	return nil
}

func (e *Entity) applyOptionalFields(in UpdateInput) {
	if in.BBCType != nil {
		e.bbcType = *in.BBCType
	}
	if in.NoOfBob != nil {
		e.noOfBob = *in.NoOfBob
	}
	if in.Notes != nil {
		e.notes = *in.Notes
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
	if in.BbnReuse != nil {
		e.bbnReuse = in.BbnReuse
	}
	if in.BoxReuse != nil {
		e.boxReuse = in.BoxReuse
	}
	if in.BoxCost != nil {
		e.boxCost = in.BoxCost
	}
	if in.BobinCost != nil {
		e.bobinCost = in.BobinCost
	}
	if in.BoxCostVal != nil {
		e.boxCostVal = in.BoxCostVal
	}
	if in.BobinCostVal != nil {
		e.bobinCostVal = in.BobinCostVal
	}
}

// =============================================================================
// RateEntry Getters
// =============================================================================

// ID returns the rate entry UUID primary key.
func (r *RateEntry) ID() uuid.UUID { return r.id }

// ParentID returns the parent entity UUID.
func (r *RateEntry) ParentID() uuid.UUID { return r.parentID }

// Period returns the period identifier (YYYYMM).
func (r *RateEntry) Period() string { return r.period }

// BobRateMkt returns the bobbin market rate.
func (r *RateEntry) BobRateMkt() float64 { return r.bobRateMkt }

// BoxRateMkt returns the box market rate.
func (r *RateEntry) BoxRateMkt() float64 { return r.boxRateMkt }

// BobRateVal returns the optional bobbin valuation rate.
func (r *RateEntry) BobRateVal() *float64 { return r.bobRateVal }

// BoxRateVal returns the optional box valuation rate.
func (r *RateEntry) BoxRateVal() *float64 { return r.boxRateVal }

// CreatedAt returns the creation timestamp.
func (r *RateEntry) CreatedAt() time.Time { return r.createdAt }

// CreatedBy returns the creator.
func (r *RateEntry) CreatedBy() string { return r.createdBy }

// UpdatedAt returns the last update timestamp.
func (r *RateEntry) UpdatedAt() *time.Time { return r.updatedAt }

// UpdatedBy returns the last updater.
func (r *RateEntry) UpdatedBy() *string { return r.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (r *RateEntry) DeletedAt() *time.Time { return r.deletedAt }

// DeletedBy returns who soft-deleted the record.
func (r *RateEntry) DeletedBy() *string { return r.deletedBy }
