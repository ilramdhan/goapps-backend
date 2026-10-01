package costcalc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// LoadUpstreamParamSnapshots implements ProductLoader.LoadUpstreamParamSnapshots.
//
// The row filter is exactly LoadUpstreamCosts's (same period, same calc type,
// cpc_status <> 'SUPERSEDED'), so the snapshot a downstream stage inherits its
// VB loss from is the same committed row its RM cost comes from. The partial
// unique index uk_cpc_active (000228) admits at most one such row per
// (product, period, calc_type); DISTINCT ON ... ORDER BY cpc_version DESC (as in
// LoadSellingSnapshots) is kept as a belt-and-braces tie-break.
//
// A NULL or unparseable snapshot leaves the product absent, which the engine
// treats like a missing upstream (value 0 + warning), never as an error.
func (l *productLoader) LoadUpstreamParamSnapshots(ctx context.Context, productSysIDs []int64, period, calcType string) (map[int64]map[string]float64, error) {
	defer observeLoad(loaderKindUpstreamSnap, time.Now())
	out := map[int64]map[string]float64{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	if period == "" || calcType == "" {
		return nil, errors.New("LoadUpstreamParamSnapshots: period and calcType are required")
	}
	const q = `
		SELECT DISTINCT ON (cpc_product_sys_id)
			cpc_product_sys_id, cpc_param_snapshot
		FROM cst_product_cost
		WHERE cpc_product_sys_id = ANY($1)
		  AND cpc_period = $2
		  AND cpc_calculation_type = $3
		  AND cpc_status <> 'SUPERSEDED'
		ORDER BY cpc_product_sys_id, cpc_version DESC`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(productSysIDs), period, calcType)
	if err != nil {
		return nil, fmt.Errorf("load upstream param snapshots: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			snapshot     []byte
		)
		if err := rows.Scan(&productSysID, &snapshot); err != nil {
			return nil, fmt.Errorf("scan upstream param snapshot: %w", err)
		}
		if len(snapshot) == 0 || string(snapshot) == "null" {
			continue
		}
		parsed := map[string]float64{}
		if jErr := json.Unmarshal(snapshot, &parsed); jErr == nil {
			out[productSysID] = parsed
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream param snapshot rows: %w", err)
	}
	return out, nil
}

// LoadProductTypeCodes implements ProductLoader.LoadProductTypeCodes with one
// query per chunk: cost_product_master.cpm_product_type_id ->
// cost_product_type.cpt_type_code. Both are NOT NULL (000106), so every
// existing product resolves; an unknown id is simply absent.
func (l *productLoader) LoadProductTypeCodes(ctx context.Context, productSysIDs []int64) (map[int64]string, error) {
	defer observeLoad(loaderKindProductType, time.Now())
	out := map[int64]string{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT pm.cpm_product_sys_id, pt.cpt_type_code
		FROM cost_product_master pm
		JOIN cost_product_type   pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pm.cpm_product_sys_id = ANY($1)`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load product type codes: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			typeCode     string
		)
		if err := rows.Scan(&productSysID, &typeCode); err != nil {
			return nil, fmt.Errorf("scan product type code: %w", err)
		}
		out[productSysID] = strings.TrimSpace(typeCode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product type codes: %w", err)
	}
	return out, nil
}
