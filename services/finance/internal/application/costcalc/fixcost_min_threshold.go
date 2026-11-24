package costcalc

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// loadFixCostMinThresholdsQuery resolves, per product, the fixed cost floor
// per kg (000615): the product's cpm_fixcost_min_threshold, else the
// threshold of its MC_NAME machine (same match and tie-break as
// loadMachineParamDefaultsQuery). Products with neither are not returned.
const loadFixCostMinThresholdsQuery = `
	SELECT pm.cpm_product_sys_id,
	       COALESCE(pm.cpm_fixcost_min_threshold, mc.thr)::float8
	FROM cost_product_master pm
	LEFT JOIN LATERAL (
	    SELECT m.mc_fixcost_min_threshold AS thr
	    FROM cost_product_parameter cpp
	    JOIN mst_parameter p
	         ON p.id = cpp.cpp_param_id
	        AND p.param_code = 'MC_NAME'
	        AND p.deleted_at IS NULL
	    JOIN mst_machine m
	         ON m.deleted_at IS NULL
	        AND m.mc_fixcost_min_threshold IS NOT NULL
	        AND (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))
	             OR LOWER(BTRIM(m.mc_code)) = LOWER(BTRIM(cpp.cpp_value_text)))
	    WHERE cpp.cpp_product_sys_id = pm.cpm_product_sys_id
	      AND NULLIF(BTRIM(cpp.cpp_value_text), '') IS NOT NULL
	    ORDER BY (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))) DESC, m.mc_code
	    LIMIT 1
	) mc ON TRUE
	WHERE pm.cpm_product_sys_id = ANY($1)
	  AND COALESCE(pm.cpm_fixcost_min_threshold, mc.thr) IS NOT NULL`

// LoadFixCostMinThresholds implements ProductLoader.LoadFixCostMinThresholds:
// productSysID -> fixed cost floor per kg. Products without a threshold are
// absent.
func (l *productLoader) LoadFixCostMinThresholds(ctx context.Context, productSysIDs []int64) (map[int64]float64, error) {
	defer observeLoad(loaderKindFixCostMin, time.Now())
	out := map[int64]float64{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := l.db.QueryContext(ctx, loadFixCostMinThresholdsQuery, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load fixed cost min thresholds: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			pid int64
			val float64
		)
		if err := rows.Scan(&pid, &val); err != nil {
			return nil, fmt.Errorf("scan fixed cost min threshold row: %w", err)
		}
		out[pid] = val
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fixed cost min threshold rows: %w", err)
	}
	return out, nil
}
