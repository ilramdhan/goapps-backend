-- Reverse 000560: restore the 000510 conversion expressions, drop the six edges
-- this migration added, re-activate F_YARN_SPECIAL_COST_2 and make SPECIAL_COST_2
-- CALCULATED again.
--
-- NOT reverted on purpose: F_YARN_WASTE_LESS_MB_OPU keeps the
-- (RM_LANDED_COST * RM_NORMS) - RM_RATE expression and its RM_* edges; that is
-- the valid prod state (set via web UI), not something this migration invented.
-- Its updated_by tag is left as-is.
--
-- Edges are deleted first, while the marker still identifies the formulas.

BEGIN;

DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id
  AND fp.param_id = p.id
  AND p.param_code IN ('WASTE_LESS_MB_OPU', 'HEATSET_COST_PER_KG', 'SPECIAL_COST_2',
                       'STEAM_COST_CNG', 'SOFTNER_COST', 'WASHING_COST')
  AND f.formula_code IN ('F_YARN_CONV_CAP', 'F_YARN_CONV_DEL')
  AND f.deleted_at IS NULL
  AND f.updated_by = 'conv_ex_mb_terms_000560';

UPDATE mst_formula
SET expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN',
    updated_at = NOW(),
    updated_by = 'wire_oil_gain_000510'
WHERE formula_code = 'F_YARN_CONV_CAP'
  AND deleted_at IS NULL
  AND updated_by = 'conv_ex_mb_terms_000560'
  AND expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + WASTE_LESS_MB_OPU + HEATSET_COST_PER_KG + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + SPECIAL_COST_2 + STEAM_COST_CNG + SOFTNER_COST + WASHING_COST + OIL_GAIN';

UPDATE mst_formula
SET expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN',
    updated_at = NOW(),
    updated_by = 'wire_oil_gain_000510'
WHERE formula_code = 'F_YARN_CONV_DEL'
  AND deleted_at IS NULL
  AND updated_by = 'conv_ex_mb_terms_000560'
  AND expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + WASTE_LESS_MB_OPU + HEATSET_COST_PER_KG + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + SPECIAL_COST_2 + STEAM_COST_CNG + SOFTNER_COST + WASHING_COST + OIL_GAIN';

-- Param first (its guard keys on the formula still carrying the marker).
UPDATE mst_parameter
SET param_category = 'CALCULATED',
    default_value = NULL,
    updated_at = NOW(),
    updated_by = NULL
WHERE param_code = 'SPECIAL_COST_2'
  AND deleted_at IS NULL
  AND updated_by = 'conv_ex_mb_terms_000560';

UPDATE mst_formula
SET is_active = TRUE,
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_SPECIAL_COST_2'
  AND deleted_at IS NULL
  AND updated_by = 'conv_ex_mb_terms_000560';

COMMIT;
