BEGIN;
-- Restore trigger params to INPUT
UPDATE mst_parameter SET param_category = 'INPUT'
WHERE param_code IN ('MC_NAME','INTERMINGLE','STD_LOSS_GRADE','BC_LOSS_GRADE','MB_CODE','CAP_PACK_CODE','DEL_PACK_CODE')
  AND deleted_at IS NULL;
-- Clear fill-group columns
UPDATE mst_parameter SET lookup_fill_group_code = NULL, lookup_source_column = NULL
WHERE lookup_fill_group_code IS NOT NULL OR lookup_source_column IS NOT NULL;
-- Remove columns
ALTER TABLE mst_parameter DROP COLUMN IF EXISTS lookup_fill_group_code;
ALTER TABLE mst_parameter DROP COLUMN IF EXISTS lookup_source_column;
-- Restore original CHECK constraint
ALTER TABLE mst_parameter DROP CONSTRAINT IF EXISTS mst_parameter_param_category_check;
ALTER TABLE mst_parameter ADD CONSTRAINT mst_parameter_param_category_check
    CHECK (param_category IN ('INPUT', 'RATE', 'CALCULATED'));
COMMIT;
