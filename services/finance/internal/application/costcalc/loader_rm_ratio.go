package costcalc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
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
// unknown product id is simply absent, which the engine treats as INPUT.
func (l *productLoader) LoadProductRMRatioModes(ctx context.Context, productSysIDs []int64) (map[int64]string, error) {
	defer observeLoad(loaderKindRMRatioMode, time.Now())
	out := map[int64]string{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT pm.cpm_product_sys_id, pt.cpt_rm_ratio_mode
		FROM cost_product_master pm
		JOIN cost_product_type   pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pm.cpm_product_sys_id = ANY($1)`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load product RM ratio modes: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			mode         string
		)
		if err := rows.Scan(&productSysID, &mode); err != nil {
			return nil, fmt.Errorf("scan product RM ratio mode: %w", err)
		}
		out[productSysID] = strings.ToUpper(strings.TrimSpace(mode))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product RM ratio modes: %w", err)
	}
	return out, nil
}

// LoadRMGroupDeniers implements ProductLoader.LoadRMGroupDeniers with one
// query per chunk: cst_rm_group_head.group_code -> denier (000570). Groups
// whose denier is NULL, soft-deleted groups and unknown codes are absent, so
// presence in the map is what makes a GROUP line a denier line.
func (l *productLoader) LoadRMGroupDeniers(ctx context.Context, groupCodes []string) (map[string]float64, error) {
	defer observeLoad(loaderKindRMGroupDenier, time.Now())
	out := map[string]float64{}
	if len(groupCodes) == 0 {
		return out, nil
	}
	const q = `
		SELECT group_code, denier::float8
		FROM cst_rm_group_head
		WHERE group_code = ANY($1)
		  AND deleted_at IS NULL
		  AND denier IS NOT NULL`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(groupCodes))
	if err != nil {
		return nil, fmt.Errorf("load RM group deniers: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			code   string
			denier float64
		)
		if err := rows.Scan(&code, &denier); err != nil {
			return nil, fmt.Errorf("scan RM group denier: %w", err)
		}
		out[code] = denier
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RM group deniers: %w", err)
	}
	return out, nil
}
