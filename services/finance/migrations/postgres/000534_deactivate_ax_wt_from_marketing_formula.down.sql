-- 000534 down: reactivate F_YARN_AX_WT_FROM_MKT with its exact prior
-- is_active/updated_at/updated_by from bak_ax_wt_formula_000534, only on the
-- row this migration deactivated (marker updated_by = 'ax_wt_input_000534').

BEGIN;

UPDATE mst_formula f
SET is_active  = b.is_active,
    updated_at = b.updated_at,
    updated_by = b.updated_by
FROM bak_ax_wt_formula_000534 b
WHERE f.formula_code = b.formula_code
  AND f.deleted_at IS NULL
  AND f.updated_by = 'ax_wt_input_000534';

DROP TABLE IF EXISTS bak_ax_wt_formula_000534;

COMMIT;
