-- Add the missing legacy terms to the yarn conversion build-up
-- (F_YARN_CONV_CAP / F_YARN_CONV_DEL -> ONLY_CONV_*_PACK_EXCL_MB), CSV rows 92/93.
--
-- LEGACY FORMULA (confirmed by Finance):
--   row 92 = cap_pack + waste_less_mb_doz_opu + heat_set_cost_per_kg + oil_cost
--          + intermingling + spl_cost_1 + spl_cost_2 + steam_cost_cng
--          + softener_cost + washing_cost + total_91 + oil_gain
--   total_91 = TOTAL_FIXEDCOST_PER_KG; row 93 is identical with del_pack.
-- 000510 left six of these terms out. This migration adds them.
--
-- KNOWN STUBS: STEAM_COST_CNG (F_YARN_STEAM_COST) and WASHING_COST
-- (F_YARN_WASHING_COST) are still the seeded literal '0' (000408:60-61), so they
-- contribute 0 for now; real rules arrive in a later migration.
--
-- PART 1  Sync F_YARN_WASTE_LESS_MB_OPU to the state prod already has (edited
--   via the web UI): (RM_LANDED_COST * RM_NORMS) - RM_RATE. Guarded on the exact
--   000408 seed text, so on prod it is a no-op. The old edges (WASTE_PERC,
--   MB_SP_DOZING, OPU) are NOT deleted (additive-only policy); stale extra edges
--   only add ordering constraints, are harmless and create no cycle. New edges
--   use sort_order 4..6 (sort_order is not part of idx_formula_param_unique,
--   which is (formula_id, param_id); 4..6 just avoids visual collision).
-- PART 2  SPECIAL_COST_2 becomes an independent INPUT (legacy spl_cost_2 may
--   differ from spl_cost_1): F_YARN_SPECIAL_COST_2 (expression 'SPECIAL_COST_1')
--   is deactivated and the param becomes INPUT with default 0, mirroring
--   SOFTNER_COST (000407:61). The per-product formula loader filters
--   f.is_active = TRUE (loader.go:808), and buildInitialScope zero-fills inputs
--   with no stored product value, so it resolves to the stored value or 0.
-- PART 3  Rewrite both conversion expressions and add six input edges
--   (sort_order 7..12). No cycle: WASTE_LESS_MB_OPU depends on RM_* (RM_RATE is a
--   route lookup, RM_NORMS depends on WASTE_PERC); HEATSET/SOFTNER/SPECIAL_COST_2/
--   STEAM/WASHING depend on no conversion or downstream param.
--
-- Every statement is guarded; re-running is a no-op.

BEGIN;

-- PART 1
UPDATE mst_formula f
SET expression = '(RM_LANDED_COST * RM_NORMS) - RM_RATE',
    updated_at = NOW(),
    updated_by = 'conv_ex_mb_terms_000560'
WHERE f.formula_code = 'F_YARN_WASTE_LESS_MB_OPU'
  AND f.deleted_at IS NULL
  AND f.expression = '(1.0 - WASTE_PERC / 100.0) - (MB_SP_DOZING / 100.0) - (OPU / 100.0)';

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so
FROM mst_formula f
JOIN (VALUES ('RM_LANDED_COST', 4), ('RM_NORMS', 5), ('RM_RATE', 6)) AS v(pc, so) ON TRUE
JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_WASTE_LESS_MB_OPU'
  AND f.deleted_at IS NULL
  AND f.updated_by = 'conv_ex_mb_terms_000560'
  AND NOT EXISTS (
      SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id
  );

-- PART 2
UPDATE mst_formula
SET is_active = FALSE,
    updated_at = NOW(),
    updated_by = 'conv_ex_mb_terms_000560'
WHERE formula_code = 'F_YARN_SPECIAL_COST_2'
  AND deleted_at IS NULL
  AND expression = 'SPECIAL_COST_1';

UPDATE mst_parameter
SET param_category = 'INPUT',
    default_value = 0,
    updated_at = NOW(),
    updated_by = 'conv_ex_mb_terms_000560'
WHERE param_code = 'SPECIAL_COST_2'
  AND deleted_at IS NULL
  AND param_category = 'CALCULATED'
  AND EXISTS (
      SELECT 1 FROM mst_formula f
      WHERE f.formula_code = 'F_YARN_SPECIAL_COST_2'
        AND f.updated_by = 'conv_ex_mb_terms_000560'
        AND f.is_active = FALSE
  );

-- PART 3
UPDATE mst_formula f
SET expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + WASTE_LESS_MB_OPU + HEATSET_COST_PER_KG + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + SPECIAL_COST_2 + STEAM_COST_CNG + SOFTNER_COST + WASHING_COST + OIL_GAIN',
    updated_at = NOW(),
    updated_by = 'conv_ex_mb_terms_000560'
WHERE f.formula_code = 'F_YARN_CONV_CAP'
  AND f.deleted_at IS NULL
  AND f.expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id AND p.param_code = 'ONLY_CONV_CAP_PACK_EXCL_MB' AND p.deleted_at IS NULL
  );

UPDATE mst_formula f
SET expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + WASTE_LESS_MB_OPU + HEATSET_COST_PER_KG + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + SPECIAL_COST_2 + STEAM_COST_CNG + SOFTNER_COST + WASHING_COST + OIL_GAIN',
    updated_at = NOW(),
    updated_by = 'conv_ex_mb_terms_000560'
WHERE f.formula_code = 'F_YARN_CONV_DEL'
  AND f.deleted_at IS NULL
  AND f.expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id AND p.param_code = 'ONLY_CONV_DEL_PACK_EXCL_MB' AND p.deleted_at IS NULL
  );

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so
FROM mst_formula f
JOIN (VALUES ('WASTE_LESS_MB_OPU', 7), ('HEATSET_COST_PER_KG', 8), ('SPECIAL_COST_2', 9),
             ('STEAM_COST_CNG', 10), ('SOFTNER_COST', 11), ('WASHING_COST', 12)) AS v(pc, so) ON TRUE
JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
WHERE f.formula_code IN ('F_YARN_CONV_CAP', 'F_YARN_CONV_DEL')
  AND f.deleted_at IS NULL
  AND f.updated_by = 'conv_ex_mb_terms_000560'
  AND NOT EXISTS (
      SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id
  );

COMMIT;
