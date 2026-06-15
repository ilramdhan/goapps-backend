-- 000379: add lock tracking columns to cost_route_head.
ALTER TABLE cost_route_head
    ADD COLUMN IF NOT EXISTS crh_locked_by    VARCHAR(64),
    ADD COLUMN IF NOT EXISTS crh_locked_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS crh_unlocked_by  VARCHAR(64),
    ADD COLUMN IF NOT EXISTS crh_unlocked_at  TIMESTAMPTZ;
