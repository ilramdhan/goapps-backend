-- 000427 DOWN: Revert bulk-promoted routes back to DRAFT.
-- WARNING: This will also revert routes that were manually completed before this migration.
-- Use with caution — only run if you need to re-promote selectively.
BEGIN;

UPDATE public.cost_route_head
SET crh_routing_status = 'DRAFT',
    crh_updated_at     = NOW(),
    crh_updated_by     = 'bulk_promote_000427_rollback'
WHERE crh_routing_status = 'COMPLETE'
  AND crh_updated_by = 'bulk_promote_000427'
  AND crh_deleted_at IS NULL;

COMMIT;
