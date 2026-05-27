-- 000241 down: soft-delete formulas seeded with created_by='seed_000241'.
-- formula_param rows are left in place (orphaned) but harmless because the
-- calc engine filters formulas by is_active.

BEGIN;

UPDATE mst_formula
   SET is_active = FALSE,
       deleted_at = NOW(),
       deleted_by = 'seed_000241'
 WHERE created_by = 'seed_000241'
   AND deleted_at IS NULL;

COMMIT;
