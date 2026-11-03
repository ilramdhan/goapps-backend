-- 000605 down - remove only rows carrying the 000605 markers (FK order: CAPP, edges, formulas, params).
-- Soft delete for formulas/params, mirroring 000561 down.

BEGIN;

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_route_total_conv_000605';

DELETE FROM formula_param fp
USING mst_formula f
WHERE fp.formula_id = f.id
  AND f.created_by = 'seed_000605'
  AND f.formula_code IN ('F_YARN_CAP_CONV_STAGE','F_YARN_DEL_CONV_STAGE','F_YARN_TOTAL_CAP_CONV','F_YARN_TOTAL_DEL_CONV');

UPDATE mst_formula
SET deleted_at = NOW(), deleted_by = 'migration_000605_down'
WHERE created_by = 'seed_000605'
  AND formula_code IN ('F_YARN_CAP_CONV_STAGE','F_YARN_DEL_CONV_STAGE','F_YARN_TOTAL_CAP_CONV','F_YARN_TOTAL_DEL_CONV')
  AND deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(), deleted_by = 'migration_000605_down'
WHERE created_by = 'seed_000605'
  AND param_code IN ('CAP_CONV_STAGE','DEL_CONV_STAGE','UPSTREAM_TOTAL_CAP_CONV','TOTAL_CAP_CONV','TOTAL_DEL_CONV')
  AND deleted_at IS NULL;

COMMIT;
