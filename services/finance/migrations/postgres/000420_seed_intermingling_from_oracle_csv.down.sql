-- 000420 DOWN: Remove Oracle CSV seed data for mst_intermingling.
BEGIN;
DELETE FROM public.mst_intermingling;
COMMIT;
