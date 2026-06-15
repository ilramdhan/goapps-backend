-- 000379 down: remove lock tracking columns.
ALTER TABLE cost_route_head
    DROP COLUMN IF EXISTS crh_locked_by,
    DROP COLUMN IF EXISTS crh_locked_at,
    DROP COLUMN IF EXISTS crh_unlocked_by,
    DROP COLUMN IF EXISTS crh_unlocked_at;
