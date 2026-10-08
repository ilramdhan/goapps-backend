-- 000561 — Cap-Pack cost (row 42, CAPTIVE_PACK_COST) defaults to a constant for POY.
--
-- Requirement (Finance): for POY products CAPTIVE_PACK_COST = 0.0078 (USD/kg);
-- every other product keeps the box/bobbin formula. Mirrors 000523/000524
-- (OIL_GAIN_POY_DEFAULT):
--   param   CAP_PACK_POY_DEFAULT            (CALCULATED, USD)
--   formula F_YARN_CAP_PACK_POY_DEFAULT     (CONSTANT) = 0.0078 -> CAP_PACK_POY_DEFAULT
--   formula F_YARN_CAP_PACK (CALCULATION) becomes
--     IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0)
-- IS_POY is engine-injected (costcalc/oil_rate.go: injectProductClassFlags), not an
-- mst_parameter row. F_YARN_DEL_PACK (delivery) is NOT touched.
--
-- HOW TO CHANGE THE VALUE: edit the expression of F_YARN_CAP_PACK_POY_DEFAULT in
-- Master Formula (web); no code change. HOW TO REVERT POY TO THE FORMULA: edit
-- F_YARN_CAP_PACK on the web to drop the "IS_POY == 1 ? ... :" branch and add a
-- follow-up migration to sync (guarded on the exact text, see 000524/000560).
--
-- Guards (000524 pattern): the UPDATE pins the exact live expression and the
-- result param CAPTIVE_PACK_COST, so a hand-edited formula is never clobbered.
-- The new edge is linked only to a formula this migration rewrote (updated_by
-- marker). Existing 000408 edges already cover CAPTIVE_NO_OF_BOB / BOB_RATE /
-- BOX_RATE / BOX_WT. Formulas only run for products whose CAPP contains the
-- result param, so CAP_PACK_POY_DEFAULT is attached to POY-class products
-- (marker seed_cap_pack_poy_000561, same relational scope as 000525).

BEGIN;

-- PART 1: new param
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, is_active, created_at, created_by
)
SELECT
    'CAP_PACK_POY_DEFAULT', 'Cap-Pack Cost POY Default', 'Cap-Pack POY Default', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'Packing', 46,
    'CAPTIVE_PACK_COST value for POY products; produced by CONSTANT formula F_YARN_CAP_PACK_POY_DEFAULT (editable in Master Formula).',
    TRUE, NOW(), 'seed_000561'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'CAP_PACK_POY_DEFAULT' AND deleted_at IS NULL
);

-- PART 2: CONSTANT formula
INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT 'F_YARN_CAP_PACK_POY_DEFAULT', 'Cap-Pack Cost POY Default', 'CONSTANT', '0.0078',
       p.id,
       'Captive pack cost for POY products (USD/kg). Editable constant used by F_YARN_CAP_PACK.',
       1, TRUE, NOW(), 'seed_000561'
FROM mst_parameter p
WHERE p.param_code = 'CAP_PACK_POY_DEFAULT'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = 'F_YARN_CAP_PACK_POY_DEFAULT' AND deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE result_param_id = p.id AND deleted_at IS NULL
  );

-- PART 3: rewrite F_YARN_CAP_PACK (type stays CALCULATION, as F_YARN_OIL_GAIN)
UPDATE mst_formula f
SET expression  = 'IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0)',
    description = 'Captive pack cost per kg. POY: CAP_PACK_POY_DEFAULT; others: (bobbins*bob rate + box rate)/box weight.',
    updated_at  = NOW(),
    updated_by  = 'cap_pack_poy_000561'
WHERE f.formula_code = 'F_YARN_CAP_PACK'
  AND f.deleted_at IS NULL
  AND f.expression = 'CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id AND p.param_code = 'CAPTIVE_PACK_COST' AND p.deleted_at IS NULL
  )
  AND EXISTS (
      SELECT 1 FROM mst_parameter p WHERE p.param_code = 'CAP_PACK_POY_DEFAULT' AND p.deleted_at IS NULL
  );

-- PART 4: input edge (drives topo order)
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id,
       COALESCE((SELECT MAX(fp2.sort_order) FROM formula_param fp2 WHERE fp2.formula_id = f.id), 0) + 1
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'CAP_PACK_POY_DEFAULT' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_CAP_PACK'
  AND f.deleted_at IS NULL
  AND f.updated_by = 'cap_pack_poy_000561'
  AND NOT EXISTS (
      SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id
  );

-- PART 5: attach to POY-class products (formula only runs if result param is in CAPP)
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
)
SELECT pm.cpm_product_sys_id, mp.id, FALSE, NULL::INT, 'seed_cap_pack_poy_000561'
FROM cost_product_master pm
JOIN cost_product_type pt
  ON pt.cpt_type_id = pm.cpm_product_type_id
 AND pt.cpt_oil_class = 'POY'
 AND pt.cpt_type_code <> 'MB'
CROSS JOIN mst_parameter mp
WHERE mp.deleted_at IS NULL
  AND mp.param_code = 'CAP_PACK_POY_DEFAULT'
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

DO $$
DECLARE
    v_expr TEXT;
BEGIN
    SELECT expression INTO v_expr FROM mst_formula WHERE formula_code = 'F_YARN_CAP_PACK' AND deleted_at IS NULL;
    IF v_expr IS NULL THEN
        RAISE NOTICE '000561: F_YARN_CAP_PACK not found — skipped.';
    ELSIF v_expr NOT LIKE 'IS_POY == 1 ?%' THEN
        RAISE NOTICE '000561: F_YARN_CAP_PACK NOT rewritten — expression drifted: %', v_expr;
    END IF;
END $$;

COMMIT;
