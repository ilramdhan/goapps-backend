// Package spinfixedcost provides domain logic for the POY spinning fixed-cost pool master.
package spinfixedcost

import (
	"regexp"
	"time"

	"github.com/google/uuid"
)

// periodPattern mirrors the chk constraint on mst_spin_fixed_cost.msfc_period.
var periodPattern = regexp.MustCompile(`^[0-9]{6}$`)

// Entity is the aggregate root for the Spin Fixed Cost domain.
//
// One live row holds the whole monthly POY spinning fixed-cost pool for a period;
// the calc engine divides it across every POY product, so a bad or missing row
// distorts thousands of costed products at once.
type Entity struct {
	id                 uuid.UUID
	period             string
	commonPoyDenier    float64
	poyProduction      float64
	spinPowerMonth     float64
	spinManpowerMonth  float64
	spinOverheadsMonth float64
	spinConssprsMonth  float64
	isActive           bool
	createdAt          time.Time
	createdBy          string
	updatedAt          *time.Time
	updatedBy          *string
	deletedAt          *time.Time
	deletedBy          *string
}

// NewInput carries the constructor arguments for New.
type NewInput struct {
	Period             string
	CommonPoyDenier    float64
	PoyProduction      float64
	SpinPowerMonth     float64
	SpinManpowerMonth  float64
	SpinOverheadsMonth float64
	SpinConssprsMonth  float64
	CreatedBy          string
}

// New creates a new Spin Fixed Cost entity with validation.
func New(in NewInput) (*Entity, error) {
	if err := validatePeriod(in.Period); err != nil {
		return nil, err
	}
	if err := validateDivisors(in.CommonPoyDenier, in.PoyProduction); err != nil {
		return nil, err
	}
	if err := validateAmounts(
		in.SpinPowerMonth, in.SpinManpowerMonth, in.SpinOverheadsMonth, in.SpinConssprsMonth,
	); err != nil {
		return nil, err
	}
	if in.CreatedBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id:                 uuid.New(),
		period:             in.Period,
		commonPoyDenier:    in.CommonPoyDenier,
		poyProduction:      in.PoyProduction,
		spinPowerMonth:     in.SpinPowerMonth,
		spinManpowerMonth:  in.SpinManpowerMonth,
		spinOverheadsMonth: in.SpinOverheadsMonth,
		spinConssprsMonth:  in.SpinConssprsMonth,
		isActive:           true,
		createdAt:          time.Now(),
		createdBy:          in.CreatedBy,
	}, nil
}

// ReconstructInput carries the persistence fields for Reconstruct.
type ReconstructInput struct {
	ID                 uuid.UUID
	Period             string
	CommonPoyDenier    float64
	PoyProduction      float64
	SpinPowerMonth     float64
	SpinManpowerMonth  float64
	SpinOverheadsMonth float64
	SpinConssprsMonth  float64
	IsActive           bool
	CreatedAt          time.Time
	CreatedBy          string
	UpdatedAt          *time.Time
	UpdatedBy          *string
	DeletedAt          *time.Time
	DeletedBy          *string
}

// Reconstruct rebuilds a Spin Fixed Cost entity from persistence data.
func Reconstruct(in ReconstructInput) *Entity {
	return &Entity{
		id:                 in.ID,
		period:             in.Period,
		commonPoyDenier:    in.CommonPoyDenier,
		poyProduction:      in.PoyProduction,
		spinPowerMonth:     in.SpinPowerMonth,
		spinManpowerMonth:  in.SpinManpowerMonth,
		spinOverheadsMonth: in.SpinOverheadsMonth,
		spinConssprsMonth:  in.SpinConssprsMonth,
		isActive:           in.IsActive,
		createdAt:          in.CreatedAt,
		createdBy:          in.CreatedBy,
		updatedAt:          in.UpdatedAt,
		updatedBy:          in.UpdatedBy,
		deletedAt:          in.DeletedAt,
		deletedBy:          in.DeletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// Period returns the YYYYMM period. Immutable after creation.
func (e *Entity) Period() string { return e.period }

// CommonPoyDenier returns the common POY denier (a calc engine divisor).
func (e *Entity) CommonPoyDenier() float64 { return e.commonPoyDenier }

// PoyProduction returns the total POY production (a calc engine divisor).
func (e *Entity) PoyProduction() float64 { return e.poyProduction }

// SpinPowerMonth returns the monthly spinning power cost.
func (e *Entity) SpinPowerMonth() float64 { return e.spinPowerMonth }

// SpinManpowerMonth returns the monthly spinning manpower cost.
func (e *Entity) SpinManpowerMonth() float64 { return e.spinManpowerMonth }

// SpinOverheadsMonth returns the monthly spinning overheads cost.
func (e *Entity) SpinOverheadsMonth() float64 { return e.spinOverheadsMonth }

// SpinConssprsMonth returns the monthly spinning consumables and spares cost.
func (e *Entity) SpinConssprsMonth() float64 { return e.spinConssprsMonth }

// IsActive returns whether the record is active.
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

// IsDeleted returns true if the record is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// UpdateInput carries optional field mutations for Update.
// Period is deliberately absent: it is immutable after creation.
type UpdateInput struct {
	CommonPoyDenier    *float64
	PoyProduction      *float64
	SpinPowerMonth     *float64
	SpinManpowerMonth  *float64
	SpinOverheadsMonth *float64
	SpinConssprsMonth  *float64
	IsActive           *bool
}

// DeactivatesRow reports whether the update would flip the entity from active to inactive.
// Callers must run the anchor-row guard before applying such an update.
func (in UpdateInput) DeactivatesRow(current *Entity) bool {
	return in.IsActive != nil && !*in.IsActive && current.IsActive()
}

// Update applies optional field changes to the entity.
func (e *Entity) Update(in UpdateInput, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	if err := e.applyDivisors(in.CommonPoyDenier, in.PoyProduction); err != nil {
		return err
	}
	if err := e.applyAmounts(in); err != nil {
		return err
	}
	if in.IsActive != nil {
		e.isActive = *in.IsActive
	}
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the record as deleted.
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

func (e *Entity) applyDivisors(denier, production *float64) error {
	if denier != nil {
		if *denier <= 0 {
			return ErrNonPositiveDenier
		}
		e.commonPoyDenier = *denier
	}
	if production != nil {
		if *production <= 0 {
			return ErrNonPositiveProduction
		}
		e.poyProduction = *production
	}
	return nil
}

func (e *Entity) applyAmounts(in UpdateInput) error {
	assignments := []struct {
		val    *float64
		target *float64
	}{
		{in.SpinPowerMonth, &e.spinPowerMonth},
		{in.SpinManpowerMonth, &e.spinManpowerMonth},
		{in.SpinOverheadsMonth, &e.spinOverheadsMonth},
		{in.SpinConssprsMonth, &e.spinConssprsMonth},
	}
	for _, a := range assignments {
		if a.val == nil {
			continue
		}
		if *a.val < 0 {
			return ErrNegativeAmount
		}
		*a.target = *a.val
	}
	return nil
}

func validatePeriod(period string) error {
	if !periodPattern.MatchString(period) {
		return ErrInvalidPeriod
	}
	return nil
}

func validateDivisors(denier, production float64) error {
	// Both are DIVISORS in the calc engine. A zero here does not raise anywhere
	// downstream: the pool arm's divide guards return 0, which is a perfectly
	// valid number, so every POY product silently costs zero fixed cost.
	if denier <= 0 {
		return ErrNonPositiveDenier
	}
	if production <= 0 {
		return ErrNonPositiveProduction
	}
	return nil
}

func validateAmounts(amounts ...float64) error {
	for _, a := range amounts {
		if a < 0 {
			return ErrNegativeAmount
		}
	}
	return nil
}
