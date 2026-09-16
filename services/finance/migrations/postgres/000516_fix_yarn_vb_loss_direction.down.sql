-- 000516_fix_yarn_vb_loss_direction.down.sql
-- Reverse 000516: restore the EXACT expressions that 000465 set (the divide form).
--
-- SCOPE
-- Only mst_formula.expression for the 5 VBx_LOSS formulas. Nothing else was changed
-- by the up file, so nothing else is reversed here.
--
-- formula_param is deliberately NOT touched. The RM_LANDED_COST edge belongs to
-- 000465, not to 000516 — the up file neither added nor removed it, and the 000465
-- divide form restored below still consumes RM_LANDED_COST. Deleting that edge here
-- would break the expression this file is restoring. Rolling back RM_LANDED_COST is
-- 000465's own .down.sql's job.
--
-- ⚠ WARNING — READ BEFORE RUNNING
-- This restores the 000465 expression, which is the arithmetically WRONG one
-- (see the 000516 up-file header: the VOLUME_BUCKET_n_QTY columns hold 1/MT, so
-- dividing by them overstates VBx_LOSS by roughly 1/QTY^2 — VB1 ~15x up to
-- VB5 ~3596x). It is provided for rollback symmetry, not because the restored state
-- is correct.
--
-- ⚠ WARNING — this assumes the pre-000516 state really was the 000465 divide form.
-- If production had drifted to something else (e.g. the pre-000465 form
-- 'CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_n_QTY' without RM_LANDED_COST), this file
-- will NOT reproduce that state — it writes the 000465 form unconditionally.
-- Block V19-A of docs/superpowers/sql/v-19-dampak-arah-vb-loss-postgres.sql records
-- what production actually had; capture that output BEFORE applying 000516.
--
-- Stored cst_product_cost rows are not rewritten here. A recalculation is required
-- for the rollback to show up in persisted costs.

BEGIN;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_1_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_1_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'rollback_000516'
WHERE formula_code = 'F_YARN_VB1_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_2_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_2_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'rollback_000516'
WHERE formula_code = 'F_YARN_VB2_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_3_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_3_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'rollback_000516'
WHERE formula_code = 'F_YARN_VB3_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_4_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_4_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'rollback_000516'
WHERE formula_code = 'F_YARN_VB4_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_5_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_5_QTY * 1000) : 0',
    updated_at = NOW(), updated_by = 'rollback_000516'
WHERE formula_code = 'F_YARN_VB5_LOSS' AND deleted_at IS NULL;

-- Symmetric hard guard: the rollback must also be all-or-nothing.
DO $verify$
DECLARE
    n_restored INTEGER;
BEGIN
    SELECT count(*) INTO n_restored
      FROM (VALUES
        ('F_YARN_VB1_LOSS','VOLUME_BUCKET_1_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_1_QTY * 1000) : 0'),
        ('F_YARN_VB2_LOSS','VOLUME_BUCKET_2_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_2_QTY * 1000) : 0'),
        ('F_YARN_VB3_LOSS','VOLUME_BUCKET_3_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_3_QTY * 1000) : 0'),
        ('F_YARN_VB4_LOSS','VOLUME_BUCKET_4_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_4_QTY * 1000) : 0'),
        ('F_YARN_VB5_LOSS','VOLUME_BUCKET_5_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_5_QTY * 1000) : 0')
      ) AS want(fcode, expr)
     WHERE EXISTS (
        SELECT 1 FROM mst_formula f
         WHERE f.formula_code = want.fcode
           AND f.deleted_at IS NULL
           AND f.expression = want.expr
     );

    RAISE NOTICE '000516 rollback: VBx_LOSS formulas restored to the 000465 divide form = % of 5', n_restored;

    IF n_restored <> 5 THEN
        RAISE EXCEPTION '000516 rollback: only % of 5 VBx_LOSS formulas were restored to the 000465 expression — a partial rollback would leave buckets on mixed operators. Aborting.', n_restored;
    END IF;
END $verify$;

COMMIT;
