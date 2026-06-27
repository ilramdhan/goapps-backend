-- 000415 DOWN: Remove Oracle CSV seed data for mst_mb_spin.
BEGIN;
DELETE FROM public.mst_mb_spin WHERE created_by LIKE 'oracle_csv%' OR created_by = 'oracle_migration';
COMMIT;
