package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
)

var _ costimportetl.OilGroupValidator = (*CostImportStagingRepository)(nil)

// rejectOilGroupsSQL removes staged OIL_NAME rows whose value is not an oil
// group allowed for the product's type (oil-cost-rm-group D4) and records one
// stg_import_error per removed row, in a single atomic statement.
//
// The product type comes from the same job's staged product master row when
// present (a product created or re-typed by this import), otherwise from the
// existing cost_product_master row (cpm_flex_02 = legacy id, Layer 2's key).
// Types without cpt_oil_class, blank values and unknown products are left for
// the generic checks (master-lookup existence / Layer 2 resolution).
const rejectOilGroupsSQL = `
WITH cand AS (
    SELECT s.ctid AS rid, btrim(s.value_text) AS v, pt.cpt_type_id, pt.cpt_type_code
    FROM stg_import_product_parameter s
    LEFT JOIN LATERAL (
        SELECT m.product_type_code
        FROM stg_import_product_master m
        WHERE m.job_id = s.job_id AND m.legacy_oracle_sys_id = s.legacy_oracle_sys_id
        ORDER BY m.row_num DESC
        LIMIT 1
    ) sm ON TRUE
    LEFT JOIN cost_product_type spt ON spt.cpt_type_code = sm.product_type_code
    LEFT JOIN cost_product_master cpm
           ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
    JOIN cost_product_type pt
      ON pt.cpt_type_id = COALESCE(spt.cpt_type_id, cpm.cpm_product_type_id)
     AND pt.cpt_oil_class IS NOT NULL
    WHERE s.job_id = $1
      AND s.param_code = $2
      AND NULLIF(btrim(s.value_text), '') IS NOT NULL
),
allowed AS (
    SELECT og.cptog_type_id AS type_id, v.group_code
    FROM cost_product_type_oil_group og
    JOIN v_rm_group_oil v ON v.group_head_id = og.cptog_group_head_id AND v.deleted_at IS NULL
),
bad AS (
    SELECT c.rid, c.v, c.cpt_type_code,
           COALESCE((SELECT string_agg(a.group_code, ', ' ORDER BY a.group_code)
                     FROM allowed a WHERE a.type_id = c.cpt_type_id), '') AS allowed_list
    FROM cand c
    WHERE NOT EXISTS (SELECT 1 FROM allowed a WHERE a.type_id = c.cpt_type_id AND a.group_code = c.v)
),
del AS (
    DELETE FROM stg_import_product_parameter s
    USING bad b
    WHERE s.ctid = b.rid
    RETURNING s.job_id, s.row_num, s.legacy_oracle_sys_id, b.v, b.cpt_type_code, b.allowed_list
)
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT d.job_id, $3, d.row_num, d.legacy_oracle_sys_id,
       'OIL_NAME "' || d.v || '" is not allowed for product type ' || d.cpt_type_code ||
       '; allowed: ' || d.allowed_list
FROM del d`

// RejectDisallowedOilGroups removes staged OIL_NAME values not allowed for the
// product's type and records a stg_import_error per removed row. Returns the
// number of rows removed.
func (r *CostImportStagingRepository) RejectDisallowedOilGroups(ctx context.Context, jobID int64) (int, error) {
	res, err := r.db.ExecContext(ctx, rejectOilGroupsSQL, jobID, oilNameParamCode, errSheetProductParam)
	if err != nil {
		return 0, fmt.Errorf("reject disallowed oil groups: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("oil group reject rows affected: %w", err)
	}
	return clampRowsAffected(n), nil
}

// oilNameParamCode is the mst_parameter code validated against the oil-group mapping.
const oilNameParamCode = "OIL_NAME"
