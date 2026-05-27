-- 000242_backfill_capp_for_formula_inputs.down.sql
-- Revert by removing rows created by this backfill (tag-scoped delete).

BEGIN;

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_000242_backfill';

COMMIT;
