-- 000246_backfill_rate_capp_values.down.sql

BEGIN;
DELETE FROM cost_product_parameter WHERE cpp_created_by = 'seed_000246_backfill';
COMMIT;
