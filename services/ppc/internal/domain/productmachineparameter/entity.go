// Package productmachineparameter provides domain logic for the PPC
// product-machine-parameter master (per product+machine typed parameter values).
package productmachineparameter

import (
	"time"

	"github.com/google/uuid"
)

// Parameter is the aggregate root for the product-machine-parameter master. It
// holds a single typed value (numeric, text or flag) for a (product, machine,
// param) triple, referencing a mst_parameter definition owned by Costing.
type Parameter struct {
	id              int64
	cpmProductSysID int64
	machineID       int64
	machineNo       string // denormalized from machine join
	paramID         string // UUID, soft ref mst_parameter (finance)
	valueNum        *float64
	valueText       *string
	valueFlag       *bool
	updatedAt       *time.Time
}

// NewParameter creates a new Parameter with validation.
func NewParameter(
	cpmProductSysID int64,
	machineID int64,
	paramID string,
	valueNum *float64,
	valueText *string,
	valueFlag *bool,
) (*Parameter, error) {
	// cpm_product_sys_id existence/active validation is done at the application
	// layer via financeclient.ValidateProduct; the domain only enforces presence.
	if cpmProductSysID <= 0 {
		return nil, ErrInvalidProduct
	}
	if machineID <= 0 {
		return nil, ErrInvalidMachine
	}
	if _, err := uuid.Parse(paramID); err != nil {
		return nil, ErrInvalidParam
	}

	return &Parameter{
		cpmProductSysID: cpmProductSysID,
		machineID:       machineID,
		paramID:         paramID,
		valueNum:        valueNum,
		valueText:       valueText,
		valueFlag:       valueFlag,
	}, nil
}

// Reconstruct rebuilds a Parameter from persistence (no validation).
func Reconstruct(
	id int64,
	cpmProductSysID int64,
	machineID int64,
	machineNo string,
	paramID string,
	valueNum *float64,
	valueText *string,
	valueFlag *bool,
	updatedAt *time.Time,
) *Parameter {
	return &Parameter{
		id:              id,
		cpmProductSysID: cpmProductSysID,
		machineID:       machineID,
		machineNo:       machineNo,
		paramID:         paramID,
		valueNum:        valueNum,
		valueText:       valueText,
		valueFlag:       valueFlag,
		updatedAt:       updatedAt,
	}
}

// ID returns the parameter identifier.
func (p *Parameter) ID() int64 { return p.id }

// CpmProductSysID returns the soft reference to the finance CPM product system ID.
func (p *Parameter) CpmProductSysID() int64 { return p.cpmProductSysID }

// MachineID returns the machine identifier.
func (p *Parameter) MachineID() int64 { return p.machineID }

// MachineNo returns the denormalized machine number.
func (p *Parameter) MachineNo() string { return p.machineNo }

// ParamID returns the parameter UUID (soft ref mst_parameter).
func (p *Parameter) ParamID() string { return p.paramID }

// ValueNum returns the numeric value, or nil when unset.
func (p *Parameter) ValueNum() *float64 { return p.valueNum }

// ValueText returns the text value, or nil when unset.
func (p *Parameter) ValueText() *string { return p.valueText }

// ValueFlag returns the boolean value, or nil when unset.
func (p *Parameter) ValueFlag() *bool { return p.valueFlag }

// UpdatedAt returns the last update timestamp.
func (p *Parameter) UpdatedAt() *time.Time { return p.updatedAt }

// Update replaces the typed value fields. Any of the value pointers may be nil
// to clear that column; the update timestamp is always refreshed.
func (p *Parameter) Update(valueNum *float64, valueText *string, valueFlag *bool) {
	p.valueNum = valueNum
	p.valueText = valueText
	p.valueFlag = valueFlag
	now := time.Now()
	p.updatedAt = &now
}
