// Package capacity provides domain logic for PPC product-machine-capacity master data.
package capacity

import "time"

// Capacity is the aggregate root for the product-machine-capacity master.
// After the v1.2 reframe it holds planning-math fields only; setup values
// (speed, positions, draw ratio) moved to the product-machine-parameter master.
type Capacity struct {
	id              int64
	cpmProductSysID int64
	machineID       int64
	machineNo       string
	prodPerDay      *float64
	efficiencyPct   *float64
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
}

// NewCapacity creates a new Capacity with validation.
func NewCapacity(
	cpmProductSysID int64,
	machineID int64,
	prodPerDay *float64,
	efficiencyPct *float64,
	createdBy string,
) (*Capacity, error) {
	// cpm_product_sys_id existence/active validation is done at the application
	// layer via financeclient.ValidateProduct; the domain only enforces presence.
	if cpmProductSysID <= 0 {
		return nil, ErrInvalidProduct
	}
	if machineID <= 0 {
		return nil, ErrInvalidMachine
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Capacity{
		cpmProductSysID: cpmProductSysID,
		machineID:       machineID,
		prodPerDay:      prodPerDay,
		efficiencyPct:   efficiencyPct,
		createdAt:       time.Now(),
		createdBy:       createdBy,
	}, nil
}

// Reconstruct rebuilds a Capacity from persistence (no validation).
func Reconstruct(
	id int64,
	cpmProductSysID int64,
	machineID int64,
	machineNo string,
	prodPerDay *float64,
	efficiencyPct *float64,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Capacity {
	return &Capacity{
		id:              id,
		cpmProductSysID: cpmProductSysID,
		machineID:       machineID,
		machineNo:       machineNo,
		prodPerDay:      prodPerDay,
		efficiencyPct:   efficiencyPct,
		createdAt:       createdAt,
		createdBy:       createdBy,
		updatedAt:       updatedAt,
		updatedBy:       updatedBy,
	}
}

// ID returns the capacity identifier.
func (c *Capacity) ID() int64 { return c.id }

// CpmProductSysID returns the soft reference to the finance CPM product system ID.
func (c *Capacity) CpmProductSysID() int64 { return c.cpmProductSysID }

// MachineID returns the machine identifier.
func (c *Capacity) MachineID() int64 { return c.machineID }

// MachineNo returns the denormalized machine number.
func (c *Capacity) MachineNo() string { return c.machineNo }

// ProdPerDay returns the planning production per day.
func (c *Capacity) ProdPerDay() *float64 { return c.prodPerDay }

// EfficiencyPct returns the planning target efficiency percentage.
func (c *Capacity) EfficiencyPct() *float64 { return c.efficiencyPct }

// CreatedAt returns the creation timestamp.
func (c *Capacity) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *Capacity) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *Capacity) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *Capacity) UpdatedBy() *string { return c.updatedBy }

// Update applies optional field changes with validation.
func (c *Capacity) Update(
	prodPerDay *float64,
	efficiencyPct *float64,
	updatedBy string,
) error {
	if prodPerDay != nil {
		c.prodPerDay = prodPerDay
	}
	if efficiencyPct != nil {
		c.efficiencyPct = efficiencyPct
	}

	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy
	return nil
}
