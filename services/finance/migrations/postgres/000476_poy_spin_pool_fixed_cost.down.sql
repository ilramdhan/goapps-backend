-- Reverse 000476.
--
-- The 4 expressions are restored to their 000408 text VERBATIM, not by stripping
-- the branch — a hand-edited rollback that drifts from 000408 would leave the
-- 9,426 non-POY products on an expression nobody has ever verified.
--
-- Order: formula_param -> expressions -> CPP -> CAPP -> mst_parameter.
-- Everything else is keyed on the 'seed_spin_pool_000476' marker, so no
-- pre-existing row is touched.

BEGIN;

-- The 12 rows added by 000476 PART 3. Rows that predate it (POWER_PER_DAY,
-- NET_PRODUCTION, NO_OF_END, ...) are matched by neither branch and survive.
DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id
  AND fp.param_id   = p.id
  AND f.formula_code IN ('F_YARN_POWER_KG','F_YARN_MANPOWER_KG','F_YARN_OVERHEAD_KG','F_YARN_SPARES_KG')
  AND p.param_code   IN ('IS_SPIN_POOL_MODEL','ACT_DENIER','MC_WEIGHTAGE');

-- Restore 000408 expressions.
UPDATE mst_formula f SET
    expression = 'NET_PRODUCTION > 0 ? POWER_PER_DAY / NET_PRODUCTION : 0',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'revert_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_POWER_KG' AND f.deleted_at IS NULL;

UPDATE mst_formula f SET
    expression = 'NET_PRODUCTION > 0 ? MANPOWER_PER_DAY / NET_PRODUCTION : 0',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'revert_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_MANPOWER_KG' AND f.deleted_at IS NULL;

UPDATE mst_formula f SET
    expression = 'NET_PRODUCTION > 0 ? OVERHEAD_PER_HEAD * NO_OF_END / NET_PRODUCTION : 0',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'revert_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_OVERHEAD_KG' AND f.deleted_at IS NULL;

UPDATE mst_formula f SET
    expression = 'NET_PRODUCTION > 0 ? SPARESCOST_PER_DAY / NET_PRODUCTION : 0',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'revert_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_SPARES_KG' AND f.deleted_at IS NULL;

DELETE FROM cost_product_parameter
WHERE cpp_created_by = 'seed_spin_pool_000476';

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_spin_pool_000476';

DELETE FROM mst_parameter
WHERE created_by = 'seed_spin_pool_000476';

COMMIT;
