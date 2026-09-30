-- 000535 down: restore the exact prior param_category/updated_at/updated_by
-- of the grade-weight params from bak_grade_wt_params_000535, only on rows this
-- migration changed (marker updated_by = 'grade_wt_calc_000535').

BEGIN;

UPDATE mst_parameter p
SET param_category = b.param_category,
    updated_at     = b.updated_at,
    updated_by     = b.updated_by
FROM bak_grade_wt_params_000535 b
WHERE p.param_code = b.param_code
  AND p.deleted_at IS NULL
  AND p.updated_by = 'grade_wt_calc_000535';

DROP TABLE IF EXISTS bak_grade_wt_params_000535;

COMMIT;
