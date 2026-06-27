-- 000417 DOWN: Remove Oracle CSV seed data for mst_mb_head.
BEGIN;
DELETE FROM public.mst_mb_head_oracle_dup_audit;
DELETE FROM public.mst_mb_head;
COMMIT;
