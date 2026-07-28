// Package productconfig provides domain logic for PPC product-config master data.
package productconfig

import "time"

// ProductConfig is the aggregate root for the product PPC-config master.
// It extends a finance CPM product with PPC planning attributes.
type ProductConfig struct {
	id               int64
	cpmProductSysID  int64
	isCommodityWatch bool
	priceSell        *float64
	machineGroupID   *int64
	yieldStd         *float64
	bufferRmPct      *float64
	axYieldPct       *float64
	denier           *float64
	createdAt        time.Time
	createdBy        string
	updatedAt        *time.Time
	updatedBy        *string
}

// NewProductConfig creates a new ProductConfig with validation.
func NewProductConfig(
	cpmProductSysID int64,
	isCommodityWatch bool,
	priceSell *float64,
	machineGroupID *int64,
	yieldStd, bufferRmPct, axYieldPct, denier *float64,
	createdBy string,
) (*ProductConfig, error) {
	// cpm_product_sys_id existence/active validation is done at the application
	// layer via financeclient.ValidateProduct; the domain only enforces presence.
	if cpmProductSysID <= 0 {
		return nil, ErrInvalidProduct
	}
	if err := validatePercentages(yieldStd, bufferRmPct, axYieldPct); err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &ProductConfig{
		cpmProductSysID:  cpmProductSysID,
		isCommodityWatch: isCommodityWatch,
		priceSell:        priceSell,
		machineGroupID:   machineGroupID,
		yieldStd:         yieldStd,
		bufferRmPct:      bufferRmPct,
		axYieldPct:       axYieldPct,
		denier:           denier,
		createdAt:        time.Now(),
		createdBy:        createdBy,
	}, nil
}

// Reconstruct rebuilds a ProductConfig from persistence (no validation).
func Reconstruct(
	id int64,
	cpmProductSysID int64,
	isCommodityWatch bool,
	priceSell *float64,
	machineGroupID *int64,
	yieldStd, bufferRmPct, axYieldPct, denier *float64,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *ProductConfig {
	return &ProductConfig{
		id:               id,
		cpmProductSysID:  cpmProductSysID,
		isCommodityWatch: isCommodityWatch,
		priceSell:        priceSell,
		machineGroupID:   machineGroupID,
		yieldStd:         yieldStd,
		bufferRmPct:      bufferRmPct,
		axYieldPct:       axYieldPct,
		denier:           denier,
		createdAt:        createdAt,
		createdBy:        createdBy,
		updatedAt:        updatedAt,
		updatedBy:        updatedBy,
	}
}

// ID returns the config identifier.
func (c *ProductConfig) ID() int64 { return c.id }

// CpmProductSysID returns the referenced finance CPM product system ID.
func (c *ProductConfig) CpmProductSysID() int64 { return c.cpmProductSysID }

// IsCommodityWatch returns whether the product is on commodity watch.
func (c *ProductConfig) IsCommodityWatch() bool { return c.isCommodityWatch }

// PriceSell returns the optional selling price.
func (c *ProductConfig) PriceSell() *float64 { return c.priceSell }

// MachineGroupID returns the optional machine-group reference.
func (c *ProductConfig) MachineGroupID() *int64 { return c.machineGroupID }

// YieldStd returns the optional standard yield.
func (c *ProductConfig) YieldStd() *float64 { return c.yieldStd }

// BufferRmPct returns the optional RM buffer percentage.
func (c *ProductConfig) BufferRmPct() *float64 { return c.bufferRmPct }

// AxYieldPct returns the optional historical %AX yield.
func (c *ProductConfig) AxYieldPct() *float64 { return c.axYieldPct }

// Denier returns the optional denier override.
func (c *ProductConfig) Denier() *float64 { return c.denier }

// CreatedAt returns the creation timestamp.
func (c *ProductConfig) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *ProductConfig) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *ProductConfig) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *ProductConfig) UpdatedBy() *string { return c.updatedBy }

// Update applies optional field changes with validation.
func (c *ProductConfig) Update(
	isCommodityWatch *bool,
	priceSell *float64,
	machineGroupID *int64,
	yieldStd, bufferRmPct, axYieldPct, denier *float64,
	updatedBy string,
) error {
	if err := validatePercentages(yieldStd, bufferRmPct, axYieldPct); err != nil {
		return err
	}

	c.applyCommodityWatch(isCommodityWatch)
	c.applyPriceSell(priceSell)
	c.applyMachineGroupID(machineGroupID)
	c.applyYieldStd(yieldStd)
	c.applyBufferRmPct(bufferRmPct)
	c.applyAxYieldPct(axYieldPct)
	c.applyDenier(denier)

	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy
	return nil
}

func (c *ProductConfig) applyCommodityWatch(v *bool) {
	if v != nil {
		c.isCommodityWatch = *v
	}
}

func (c *ProductConfig) applyPriceSell(v *float64) {
	if v != nil {
		c.priceSell = v
	}
}

func (c *ProductConfig) applyMachineGroupID(v *int64) {
	if v != nil {
		c.machineGroupID = v
	}
}

func (c *ProductConfig) applyYieldStd(v *float64) {
	if v != nil {
		c.yieldStd = v
	}
}

func (c *ProductConfig) applyBufferRmPct(v *float64) {
	if v != nil {
		c.bufferRmPct = v
	}
}

func (c *ProductConfig) applyAxYieldPct(v *float64) {
	if v != nil {
		c.axYieldPct = v
	}
}

func (c *ProductConfig) applyDenier(v *float64) {
	if v != nil {
		c.denier = v
	}
}

func validatePercentages(yieldStd, bufferRmPct, axYieldPct *float64) error {
	for _, v := range []*float64{yieldStd, bufferRmPct, axYieldPct} {
		if v != nil && *v < 0 {
			return ErrNegativePercentage
		}
	}
	return nil
}
