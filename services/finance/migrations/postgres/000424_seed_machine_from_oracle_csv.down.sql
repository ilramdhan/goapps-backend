-- 000424 DOWN: Remove Oracle CSV seed data for mst_machine.
BEGIN;
DELETE FROM public.mst_machine;
COMMIT;
