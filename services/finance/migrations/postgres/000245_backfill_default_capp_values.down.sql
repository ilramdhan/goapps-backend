-- 000245_backfill_default_capp_values.down.sql

BEGIN;
DELETE FROM cost_product_parameter WHERE cpp_created_by = 'seed_000245_backfill';
COMMIT;
