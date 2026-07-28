// Package overrun provides the over-production threshold resolver domain service
// (5-level, most-specific-wins) used at WO over-production checks.
package overrun

import "context"

// Level values, most-specific first. Resolution walks WO → PRODUCT →
// PRODUCT_TYPE → MACHINE_GROUP → SYSTEM and returns the first configured match.
const (
	LevelWO           = "WO"
	LevelProduct      = "PRODUCT"
	LevelProductType  = "PRODUCT_TYPE"
	LevelMachineGroup = "MACHINE_GROUP"
	LevelSystem       = "SYSTEM"
)

// Unit values.
const (
	UnitPct  = "PCT"
	UnitDoff = "DOFF"
)

// Status is the outcome of an over-production check.
type Status string

// Over-production statuses.
const (
	// StatusOK means actual is within the warning band.
	StatusOK Status = "OK"
	// StatusWarning means actual crossed the warning threshold.
	StatusWarning Status = "WARNING"
	// StatusBlocked means actual crossed the block threshold (needs PM override).
	StatusBlocked Status = "BLOCKED"
)

// Threshold is a resolved threshold config (warning + block, in Unit).
type Threshold struct {
	Level        string
	Unit         string
	WarningValue float64
	BlockValue   float64
}

// Scope identifies the WO context used to resolve the applicable threshold.
type Scope struct {
	WOID           int64
	ProductSysID   int64
	ProductTypeID  int64
	MachineGroupID int64
}

// ConfigLookup resolves an active threshold config for a level+ref, or nil when
// none is configured at that level. Implemented by the infrastructure layer.
type ConfigLookup interface {
	FindThreshold(ctx context.Context, level string, refID *int64) (*Threshold, error)
}

// Resolver resolves the most-specific over-production threshold for a scope.
type Resolver struct {
	lookup ConfigLookup
}

// NewResolver builds a threshold resolver over a config lookup.
func NewResolver(lookup ConfigLookup) *Resolver {
	return &Resolver{lookup: lookup}
}

// Resolve returns the most-specific configured threshold for the scope, walking
// WO → PRODUCT → PRODUCT_TYPE → MACHINE_GROUP → SYSTEM. Returns nil when no
// level is configured (caller treats that as "no fence").
func (r *Resolver) Resolve(ctx context.Context, scope Scope) (*Threshold, error) {
	candidates := []struct {
		level string
		ref   *int64
	}{
		{LevelWO, nonZero(scope.WOID)},
		{LevelProduct, nonZero(scope.ProductSysID)},
		{LevelProductType, nonZero(scope.ProductTypeID)},
		{LevelMachineGroup, nonZero(scope.MachineGroupID)},
		{LevelSystem, nil},
	}
	for _, c := range candidates {
		if c.level != LevelSystem && c.ref == nil {
			continue
		}
		th, err := r.lookup.FindThreshold(ctx, c.level, c.ref)
		if err != nil {
			return nil, err
		}
		if th != nil {
			return th, nil
		}
	}
	return nil, nil //nolint:nilnil // no configured level is a valid "no fence" result
}

// Evaluate classifies an actual quantity against a resolved threshold and the
// WO target. For PCT the thresholds are fractions/percentages over target; for
// DOFF they are absolute kilograms of allowed overrun. A nil threshold is OK.
func Evaluate(th *Threshold, target, actual float64) Status {
	if th == nil || target <= 0 {
		return StatusOK
	}
	warnLimit, blockLimit := absoluteLimits(th, target)
	switch {
	case actual > blockLimit:
		return StatusBlocked
	case actual > warnLimit:
		return StatusWarning
	default:
		return StatusOK
	}
}

// absoluteLimits converts a threshold to absolute warning/block quantities.
func absoluteLimits(th *Threshold, target float64) (warnLimit, blockLimit float64) {
	if th.Unit == UnitDoff {
		return target + th.WarningValue, target + th.BlockValue
	}
	// PCT: values may be expressed as whole percents (3, 6) or fractions.
	warn := th.WarningValue
	block := th.BlockValue
	if warn > 1 || block > 1 {
		warn /= 100
		block /= 100
	}
	return target * (1 + warn), target * (1 + block)
}

func nonZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
