package costproductmaster

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Per-product RM ratio mode (migration 000579, cpm_rm_ratio_mode).
//
// The override is nullable: "" (NULL) follows the product type's
// cpt_rm_ratio_mode (000570); INPUT or DENIER overrides it for this product
// only. The calc engine and route graph resolve
// COALESCE(cpm_rm_ratio_mode, cpt_rm_ratio_mode) (rmratioquery).

// RM ratio mode values accepted by the per-product override.
const (
	RMRatioModeInput  = "INPUT"
	RMRatioModeDenier = "DENIER"
)

// ErrInvalidRMRatioModeOverride is returned for an override other than "", INPUT or DENIER.
var ErrInvalidRMRatioModeOverride = errors.New("rm_ratio_mode_override must be empty, INPUT or DENIER")

// NormalizeRMRatioModeOverride trims and upper-cases an override and
// validates it. "" means clear (follow the product type).
func NormalizeRMRatioModeOverride(mode string) (string, error) {
	m := strings.ToUpper(strings.TrimSpace(mode))
	switch m {
	case "", RMRatioModeInput, RMRatioModeDenier:
		return m, nil
	default:
		return "", ErrInvalidRMRatioModeOverride
	}
}

// RMRatioModeOverride returns cpm_rm_ratio_mode ("" = follow product type).
func (p *CostProductMaster) RMRatioModeOverride() string { return p.rmRatioModeOverride }

// EffectiveRMRatioMode returns COALESCE(override, product type mode) as read
// from persistence ("" when not loaded).
func (p *CostProductMaster) EffectiveRMRatioMode() string { return p.effectiveRMRatioMode }

// RestoreRMRatioMode sets the override and effective mode loaded from
// persistence. It does not validate and does not touch the audit fields.
func (p *CostProductMaster) RestoreRMRatioMode(override, effective string) {
	p.rmRatioModeOverride = override
	p.effectiveRMRatioMode = effective
}

// SetRMRatioModeOverride validates and applies a new override ("" clears).
// It returns whether anything changed. Inactive products are refused. When
// the override is set, the effective mode becomes the override; when cleared,
// the effective mode is unknown until reloaded (it follows the type).
func (p *CostProductMaster) SetRMRatioModeOverride(mode, actor string, at time.Time) (bool, error) {
	if !p.isActive {
		return false, ErrInactive
	}
	m, err := NormalizeRMRatioModeOverride(mode)
	if err != nil {
		return false, err
	}
	if m == p.rmRatioModeOverride {
		return false, nil
	}
	p.rmRatioModeOverride = m
	if m != "" {
		p.effectiveRMRatioMode = m
	}
	p.updatedAt = at.UTC()
	p.updatedBy = actor
	return true, nil
}

// RMRatioModeWrite is one cpm_rm_ratio_mode write ("" = NULL).
type RMRatioModeWrite struct {
	ProductSysID int64
	Override     string
	UpdatedAt    time.Time
	UpdatedBy    string
}

// RMRatioModeRepository persists the per-product RM ratio mode. It is a
// separate port from Repository so existing implementations stay unchanged.
type RMRatioModeRepository interface {
	// SaveRMRatioMode writes cpm_rm_ratio_mode and cpm_updated_at/by only.
	// It returns ErrNotFound for an unknown product.
	SaveRMRatioMode(ctx context.Context, w RMRatioModeWrite) error
}
