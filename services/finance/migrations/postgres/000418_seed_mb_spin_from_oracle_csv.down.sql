-- 000418 DOWN: Remove Oracle CSV seed data for mst_mb_spin.
BEGIN;
DELETE FROM public.mst_mb_spin;
COMMIT;
