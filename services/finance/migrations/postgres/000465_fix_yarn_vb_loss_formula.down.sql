-- 000465_fix_yarn_vb_loss_formula.down.sql
BEGIN;

-- 1. Restore original expressions.
UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_1_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_1_QTY : 0',
    updated_at = NOW(), updated_by = 'rollback_000465'
WHERE formula_code = 'F_YARN_VB1_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_2_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_2_QTY : 0',
    updated_at = NOW(), updated_by = 'rollback_000465'
WHERE formula_code = 'F_YARN_VB2_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_3_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_3_QTY : 0',
    updated_at = NOW(), updated_by = 'rollback_000465'
WHERE formula_code = 'F_YARN_VB3_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_4_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_4_QTY : 0',
    updated_at = NOW(), updated_by = 'rollback_000465'
WHERE formula_code = 'F_YARN_VB4_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_5_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_5_QTY : 0',
    updated_at = NOW(), updated_by = 'rollback_000465'
WHERE formula_code = 'F_YARN_VB5_LOSS' AND deleted_at IS NULL;

-- 2. Remove the RM_LANDED_COST formula_param.
DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula
    WHERE formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
      AND deleted_at IS NULL
)
AND param_id = (SELECT id FROM mst_parameter WHERE param_code = 'RM_LANDED_COST' AND deleted_at IS NULL);

-- No sort_order restore needed: the up migration does not renumber (formula_param
-- has no updated_at column, and the pre-existing rows keep their 1 / 2 values).
-- Deleting the RM_LANDED_COST row above is sufficient to restore the prior state.

COMMIT;
