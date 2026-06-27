-- 000427: Bulk promote all DRAFT routes to COMPLETE for products that have params.
--
-- Routes imported from Oracle land as DRAFT. The calc engine orchestrator
-- requires COMPLETE or LOCKED status to include a product in a job.
-- Products without params are left as DRAFT (not ready for calculation).
--
-- Promotes: routes whose product has at least one cost_product_parameter row.
-- Skips:    routes whose product has no params yet (will be promoted after
--           param import completes for those products).

BEGIN;

UPDATE public.cost_route_head crh
SET crh_routing_status = 'COMPLETE',
    crh_updated_at     = NOW(),
    crh_updated_by     = 'bulk_promote_000427'
WHERE crh.crh_routing_status = 'DRAFT'
  AND crh.crh_deleted_at IS NULL
  AND EXISTS (
    SELECT 1 FROM public.cost_product_parameter
    WHERE cpp_product_sys_id = crh.crh_product_sys_id
  );

DO $$
DECLARE v_updated INTEGER;
BEGIN
  GET DIAGNOSTICS v_updated = ROW_COUNT;
  RAISE NOTICE '000427: promoted % DRAFT routes → COMPLETE (products with params)', v_updated;
END $$;

COMMIT;
