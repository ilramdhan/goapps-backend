package costproductmaster

import (
	"context"
	"errors"
	"time"
)

// Per-product fixed-cost minimum threshold (migration 000615,
// cpm_fixcost_min_threshold). nil = auto (resolved by machine); a value is the
// threshold; 0 = floor off (stored as 0, not NULL).
//
// ponytail: no reset-to-NULL path; 0 disables, a value sets. Add a clear flag
// if Finance asks.

// ErrInvalidFixCostMinThreshold is returned for a threshold outside 0..1.
var ErrInvalidFixCostMinThreshold = errors.New("fixcost_min_threshold must be between 0 and 1")

// FixCostMinThreshold returns cpm_fixcost_min_threshold (nil = auto by machine).
func (p *CostProductMaster) FixCostMinThreshold() *float64 { return p.fixcostMinThreshold }

// RestoreFixCostMinThreshold sets the threshold loaded from persistence. It
// does not validate and does not touch the audit fields.
func (p *CostProductMaster) RestoreFixCostMinThreshold(v *float64) { p.fixcostMinThreshold = v }

// SetFixCostMinThreshold validates and applies a new threshold. It returns
// whether anything changed. Inactive products are refused.
func (p *CostProductMaster) SetFixCostMinThreshold(v float64, actor string, at time.Time) (bool, error) {
	if !p.isActive {
		return false, ErrInactive
	}
	if v < 0 || v > 1 {
		return false, ErrInvalidFixCostMinThreshold
	}
	if p.fixcostMinThreshold != nil && *p.fixcostMinThreshold == v {
		return false, nil
	}
	p.fixcostMinThreshold = &v
	p.updatedAt = at.UTC()
	p.updatedBy = actor
	return true, nil
}

// FixCostMinThresholdWrite is one cpm_fixcost_min_threshold write.
type FixCostMinThresholdWrite struct {
	ProductSysID int64
	Threshold    float64
	UpdatedAt    time.Time
	UpdatedBy    string
}

// FixCostMinThresholdRepository persists the per-product threshold. It is a
// separate port from Repository so existing implementations stay unchanged.
type FixCostMinThresholdRepository interface {
	// SaveFixCostMinThreshold writes cpm_fixcost_min_threshold and
	// cpm_updated_at/by only. It returns ErrNotFound for an unknown product.
	SaveFixCostMinThreshold(ctx context.Context, w FixCostMinThresholdWrite) error
}
