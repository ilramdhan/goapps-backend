-- 000411 DOWN: Remove Oracle master data inserted by this migration.
-- Note: dev-seed placeholder rows (seed_000384, 000386, 000387) are NOT restored.
--       Re-run the relevant up migrations or re-seed manually if needed.
BEGIN;

DELETE FROM public.mst_mb_spin         WHERE created_by = 'oracle_migration';
DELETE FROM public.mst_mb_head         WHERE created_by = 'oracle_migration';
DELETE FROM public.mst_mb_head_oracle_dup_audit;
DELETE FROM public.mst_box_bobbin_cost WHERE created_by = 'oracle_migration';
DELETE FROM public.mst_machine         WHERE created_by = 'oracle_migration';
DELETE FROM public.mst_intermingling   WHERE created_by = 'oracle_migration';
DELETE FROM public.mst_product_grade   WHERE created_by = 'oracle_migration';

COMMIT;
