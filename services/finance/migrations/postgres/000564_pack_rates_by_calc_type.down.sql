-- 000564 down — restore the exact pre-migration formulas (from bak_000564_formula) and remove
-- everything 000564 added. Only formulas still carrying the marker are restored.
BEGIN;

-- edges first, while the marker still identifies the rewritten formulas
DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id AND fp.param_id = p.id
  AND f.deleted_at IS NULL AND f.updated_by = 'pack_rates_000564'
  AND p.param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL','DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT');

DO $$
BEGIN
    IF to_regclass('bak_000564_formula') IS NOT NULL THEN
        UPDATE mst_formula f
           SET expression = b.old_expression, description = b.old_description,
               updated_at = b.old_updated_at, updated_by = b.old_updated_by
          FROM bak_000564_formula b
         WHERE f.formula_code = b.formula_code AND f.deleted_at IS NULL
           AND f.updated_by = 'pack_rates_000564';
    ELSE
        RAISE NOTICE '000564 down: bak_000564_formula missing, formulas NOT restored';
    END IF;
END $$;

-- value rows / CAPP rows of the 4 params (params are removed, so no orphans are left)
DELETE FROM cost_product_parameter
 WHERE cpp_param_id IN (SELECT id FROM mst_parameter
                         WHERE param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL','DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT')
                           AND created_by = 'seed_000564');
DELETE FROM cost_product_applicable_param
 WHERE capp_param_id IN (SELECT id FROM mst_parameter
                          WHERE param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL','DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT')
                            AND created_by = 'seed_000564');

UPDATE mst_parameter
   SET deleted_at = NOW(), deleted_by = 'migration_000564_down'
 WHERE param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL','DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT')
   AND created_by = 'seed_000564' AND deleted_at IS NULL;

DROP TABLE IF EXISTS bak_000564_pack_rate_values;
DROP TABLE IF EXISTS bak_000564_formula;

COMMIT;
