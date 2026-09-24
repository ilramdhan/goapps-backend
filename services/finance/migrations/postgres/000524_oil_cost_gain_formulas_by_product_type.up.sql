-- 000524 (oil-cost-rm-group M5) — oil cost / oil gain by product class.
--
-- New expressions (spec §4.4, exact strings — asserted by the costcalc
-- migration fixture test, P3-T4):
--   F_YARN_OIL_COST
--     (IS_PTY == 1 || IS_POY == 1) ? ((OPU / (1 - WASTE_PERC / 100)) / (1 - 0.11)) * OIL_RATE / 100 : (IS_SUPERBA == 1 ? (OIL_RATE * OPU) / 1000 : 0)
--   F_YARN_OIL_GAIN
--     IS_PTY == 1 ? ((OPU * OIL_RATE) / 100) * -1 : (IS_POY == 1 ? OIL_GAIN_POY_DEFAULT : 0)
--   F_YARN_OIL_GAIN_POY_DEFAULT (NEW, CONSTANT) = -0.002  -> OIL_GAIN_POY_DEFAULT
--
-- IS_PTY / IS_POY / IS_SUPERBA are NOT mst_parameter rows: the calc engine
-- injects them (1/0 as float64) from cost_product_type.cpt_oil_class, like the
-- SPIN_* keys. OIL_RATE is injected per period from the OIL_NAME group (000523).
-- 0.11 is a fixed literal whose meaning is still open ("arti TBD", open item O-1).
-- OIL_GAIN is stored NEGATIVE and is ADDED by F_YARN_CONV_CAP / F_YARN_CONV_DEL
-- (000510) — those two formulas are not touched here (D7).
--
-- Guards (000510 pattern): each UPDATE pins the exact expression seeded by
-- 000408, so it is idempotent and never clobbers a hand-edited formula. New
-- input edges are only linked to formulas this migration actually rewrote
-- (updated_by marker), so edges and expressions cannot drift apart. A NOTICE
-- reports when a formula was skipped because its expression had drifted.

BEGIN;

-- ============================================================
-- PART 1: new CONSTANT formula F_YARN_OIL_GAIN_POY_DEFAULT
-- ============================================================
INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT 'F_YARN_OIL_GAIN_POY_DEFAULT', 'Oil Gain POY Default', 'CONSTANT', '-0.002',
       p.id,
       'Oil gain for POY products (USD/kg, stored negative). Editable constant used by F_YARN_OIL_GAIN.',
       1, TRUE, NOW(), 'seed_000524'
FROM mst_parameter p
WHERE p.param_code = 'OIL_GAIN_POY_DEFAULT'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = 'F_YARN_OIL_GAIN_POY_DEFAULT' AND deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE result_param_id = p.id AND deleted_at IS NULL
  );

-- ============================================================
-- PART 2: rewrite F_YARN_OIL_COST (guarded by the 000408 expression)
-- ============================================================
UPDATE mst_formula f
SET expression  = '(IS_PTY == 1 || IS_POY == 1) ? ((OPU / (1 - WASTE_PERC / 100)) / (1 - 0.11)) * OIL_RATE / 100 : (IS_SUPERBA == 1 ? (OIL_RATE * OPU) / 1000 : 0)',
    description = 'Oil cost per kg by product oil class. PTY/POY: ((OPU/(1-WASTE_PERC/100))/(1-0.11))*OIL_RATE/100; Superba: OIL_RATE*OPU/1000; others 0. 0.11 (arti TBD). OIL_RATE resolved per period from the OIL_NAME RM group (CR→SR→PR).',
    updated_at  = NOW(),
    updated_by  = 'oil_by_type_000524'
WHERE f.formula_code = 'F_YARN_OIL_COST'
  AND f.deleted_at IS NULL
  AND f.expression = 'OIL_RATE * OPU / 100.0'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id AND p.param_code = 'OIL_COST' AND p.deleted_at IS NULL
  )
  AND EXISTS (
      SELECT 1 FROM mst_parameter p WHERE p.param_code = 'WASTE_PERC' AND p.deleted_at IS NULL
  );

-- ============================================================
-- PART 3: rewrite F_YARN_OIL_GAIN (guarded by the 000408 expression)
-- ============================================================
UPDATE mst_formula f
SET expression  = 'IS_PTY == 1 ? ((OPU * OIL_RATE) / 100) * -1 : (IS_POY == 1 ? OIL_GAIN_POY_DEFAULT : 0)',
    description = 'Oil gain per kg (stored negative, added by conversion cost). PTY: -(OPU*OIL_RATE)/100; POY: OIL_GAIN_POY_DEFAULT; others 0.',
    updated_at  = NOW(),
    updated_by  = 'oil_by_type_000524'
WHERE f.formula_code = 'F_YARN_OIL_GAIN'
  AND f.deleted_at IS NULL
  AND f.expression = '0'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id AND p.param_code = 'OIL_GAIN' AND p.deleted_at IS NULL
  )
  AND EXISTS (
      SELECT 1 FROM mst_parameter p WHERE p.param_code = 'OIL_GAIN_POY_DEFAULT' AND p.deleted_at IS NULL
  );

-- ============================================================
-- PART 4: input edges (formula_param) — drive topo order + terminal selection
-- ============================================================
-- F_YARN_OIL_COST already has OIL_RATE(1), OPU(2) from 000408; add WASTE_PERC(3).
-- F_YARN_OIL_GAIN had none; add OPU(1), OIL_RATE(2), OIL_GAIN_POY_DEFAULT(3).
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, e.sort_order
FROM (VALUES
    ('F_YARN_OIL_COST', 'WASTE_PERC',           3),
    ('F_YARN_OIL_GAIN', 'OPU',                  1),
    ('F_YARN_OIL_GAIN', 'OIL_RATE',             2),
    ('F_YARN_OIL_GAIN', 'OIL_GAIN_POY_DEFAULT', 3)
) AS e(fcode, pcode, sort_order)
JOIN mst_formula f
  ON f.formula_code = e.fcode
 AND f.deleted_at IS NULL
 AND f.updated_by = 'oil_by_type_000524'
JOIN mst_parameter p
  ON p.param_code = e.pcode
 AND p.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM formula_param fp
    WHERE fp.formula_id = f.id AND fp.param_id = p.id
);

-- ============================================================
-- Guard report
-- ============================================================
DO $$
DECLARE
    v_cost_expr TEXT;
    v_gain_expr TEXT;
    v_poy       INT;
BEGIN
    SELECT expression INTO v_cost_expr FROM mst_formula WHERE formula_code = 'F_YARN_OIL_COST' AND deleted_at IS NULL;
    SELECT expression INTO v_gain_expr FROM mst_formula WHERE formula_code = 'F_YARN_OIL_GAIN' AND deleted_at IS NULL;
    SELECT COUNT(*) INTO v_poy FROM mst_formula WHERE formula_code = 'F_YARN_OIL_GAIN_POY_DEFAULT' AND deleted_at IS NULL;

    IF v_cost_expr IS NULL THEN
        RAISE NOTICE '000524: F_YARN_OIL_COST not found — skipped (expected on a DB without the 000408 seed).';
    ELSIF v_cost_expr NOT LIKE '(IS_PTY == 1 || IS_POY == 1)%' THEN
        RAISE NOTICE '000524: F_YARN_OIL_COST NOT rewritten — expression drifted from the 000408 seed: %', v_cost_expr;
    END IF;

    IF v_gain_expr IS NULL THEN
        RAISE NOTICE '000524: F_YARN_OIL_GAIN not found — skipped (expected on a DB without the 000408 seed).';
    ELSIF v_gain_expr NOT LIKE 'IS_PTY == 1 ?%' THEN
        RAISE NOTICE '000524: F_YARN_OIL_GAIN NOT rewritten — expression drifted from the 000408 seed: %', v_gain_expr;
    END IF;

    RAISE NOTICE '000524: F_YARN_OIL_GAIN_POY_DEFAULT present = %', v_poy;
END $$;

COMMIT;
