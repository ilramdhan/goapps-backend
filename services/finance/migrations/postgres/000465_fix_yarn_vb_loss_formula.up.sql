-- 000465_fix_yarn_vb_loss_formula.up.sql
-- Fix ENG-YARN-01: VBx_LOSS change-over formula uses a kg÷MT-unit mismatch.
-- Before: VOLUME_BUCKET_x_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_x_QTY : 0
--  CHANGE_OVER_QLTY_LOSS is in kg (kgs_lost_change, NUMERIC(10,4)).
--  VOLUME_BUCKET_x_QTY is a batch-weight threshold in MT (e.g. 0.1923 MT).
--  300 kg / 0.1923 MT ≈ 1560 kg-per-MT, NOT a per-kg cost. This is added
--  straight onto DELIVERY_COST_QLTY_LOSS in F_YARN_VBx_DEL, roughly doubling it.
-- After: (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_x_QTY * 1000)
--  This converts: kg × (USD/kg) / (MT × 1000 kg/MT) = USD / kg — a per-kg cost.
--  RM_LANDED_COST is added as the 3rd formula_param for each VBx_LOSS formula.
BEGIN;

-- 1. Fix the expression for all 5 VBx_LOSS formulas.
UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_1_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_1_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'migration_000465'
WHERE formula_code = 'F_YARN_VB1_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_2_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_2_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'migration_000465'
WHERE formula_code = 'F_YARN_VB2_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_3_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_3_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'migration_000465'
WHERE formula_code = 'F_YARN_VB3_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_4_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_4_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'migration_000465'
WHERE formula_code = 'F_YARN_VB4_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_5_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_5_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'migration_000465'
WHERE formula_code = 'F_YARN_VB5_LOSS' AND deleted_at IS NULL;

-- 2. Add RM_LANDED_COST as the 3rd formula_param (sort_order=0) for each VBx_LOSS.
--    The 2 existing params (CHANGE_OVER_QLTY_LOSS=1, VOLUME_BUCKET_x_QTY=2) are renumbered
--    to (RM_LANDED_COST=0, CHANGE_OVER_QLTY_LOSS=1, VOLUME_BUCKET_x_QTY=2) so that
--    RM_LANDED_COST evaluates first and is available for the new multiplication.
WITH vb_formulas AS (
    SELECT id, formula_code
    FROM mst_formula
    WHERE formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
      AND deleted_at IS NULL
),
landed_id AS (
    SELECT id FROM mst_parameter WHERE param_code = 'RM_LANDED_COST' AND deleted_at IS NULL
)
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, l.id, 0
FROM vb_formulas f, landed_id l
WHERE NOT EXISTS (
    SELECT 1 FROM formula_param fp
    WHERE fp.formula_id = f.id
      AND fp.param_id = l.id
);

-- NOTE: no sort_order renumbering is needed or possible.
-- formula_param has only (id, formula_id, param_id, sort_order) — no updated_at,
-- no deleted_at. The existing rows already sit at 1 (CHANGE_OVER_QLTY_LOSS) and
-- 2 (VOLUME_BUCKET_x_QTY), and RM_LANDED_COST is inserted at 0 above, so the
-- resulting order is already correct: 0, 1, 2.
--
-- sort_order does not drive evaluation order anyway — the engine topo-sorts
-- formulas by their result/input params (loader.go topoSortFormulas), so
-- RM_LANDED_COST is guaranteed to be resolved by F_YARN_RM_LANDED before any
-- VBx_LOSS reads it. sort_order only affects display order of a formula's
-- inputs in the UI.

COMMIT;
