-- 000422 DOWN: Remove Oracle CSV seed data for mst_box_bobbin_cost.
BEGIN;
DELETE FROM public.mst_box_bobbin_cost;
COMMIT;
