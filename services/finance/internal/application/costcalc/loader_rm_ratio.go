package costcalc

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres/rmratioquery"
)

// RM ratio modes (cost_product_type.cpt_rm_ratio_mode, migration 000570).
const (
	// RMRatioModeInput keeps every route RM line's crm_route_rm_ratio as
	// entered. "" is treated identically (legacy callers, e.g. mbbatch).
	RMRatioModeInput = costroute.RMRatioModeInput
	// RMRatioModeDenier derives the ratio of the denier lines (PRODUCT RMs and
	// GROUP RMs whose head has a denier) from their denier share.
	RMRatioModeDenier = costroute.RMRatioModeDenier
)

// LoadProductRMRatioModes implements ProductLoader.LoadProductRMRatioModes with
// one query per chunk: cost_product_master.cpm_product_type_id ->
// cost_product_type.cpt_rm_ratio_mode (000570, NOT NULL DEFAULT 'INPUT'). An
// unknown product id is simply absent, which the engine treats as INPUT. The
// SQL is shared with the route graph ratio loader (rmratioquery).
func (l *productLoader) LoadProductRMRatioModes(ctx context.Context, productSysIDs []int64) (map[int64]string, error) {
	defer observeLoad(loaderKindRMRatioMode, time.Now())
	return rmratioquery.ProductRatioModes(ctx, l.db, productSysIDs)
}

// LoadRMGroupDeniers implements ProductLoader.LoadRMGroupDeniers with one
// query per chunk: cst_rm_group_head.group_code -> denier (000570) and
// denier_use_draft (000571). Groups
// whose denier is NULL, soft-deleted groups and unknown codes are absent, so
// presence in the map is what makes a GROUP line a denier line.
func (l *productLoader) LoadRMGroupDeniers(ctx context.Context, groupCodes []string) (map[string]float64, map[string]bool, error) {
	defer observeLoad(loaderKindRMGroupDenier, time.Now())
	return rmratioquery.GroupDeniers(ctx, l.db, groupCodes)
}

// LoadUpstreamDeniers implements ProductLoader.LoadUpstreamDeniers with one
// query per chunk: the DENIER INPUT param (000407) of every upstream product
// from the product master (cost_product_parameter via CAPP). DENIER is never
// formula-produced, so the master value is authoritative; products without a
// value are absent (the engine treats them as 0 + note).
func (l *productLoader) LoadUpstreamDeniers(ctx context.Context, productSysIDs []int64) (map[int64]float64, error) {
	defer observeLoad(loaderKindUpstreamDenier, time.Now())
	return rmratioquery.ProductParamValues(ctx, l.db, productSysIDs, costroute.ParamCodeDenier)
}

// LoadRMDenierOverrides implements ProductLoader.LoadRMDenierOverrides with one
// query per chunk over cost_product_rm_denier_override (000578). The SQL is
// shared with the route graph ratio loader (rmratioquery).
func (l *productLoader) LoadRMDenierOverrides(ctx context.Context, productSysIDs []int64) (map[int64]costroute.DenierOverrides, error) {
	defer observeLoad(loaderKindRMDenierOvr, time.Now())
	rows, err := rmratioquery.ProductDenierOverrides(ctx, l.db, productSysIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]costroute.DenierOverrides, len(rows))
	for id, ov := range rows {
		out[id] = costroute.DenierOverrides{Product: ov.Product, Group: ov.Group}
	}
	return out, nil
}
