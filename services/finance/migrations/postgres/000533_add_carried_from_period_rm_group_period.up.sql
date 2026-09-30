-- Migration: add carried_from_period to the RM group period snapshot tables.
--
-- Carry-forward (backlog1 design, Item 3): a period with no exact snapshot row
-- inherits from the latest earlier period's row, falling back to the anchor
-- row. When calculate_handler_v2 calculates period P for a head/detail that
-- has no exact row yet, it freezes the resolved values as an exact row for P
-- (is_backfilled = true). carried_from_period records where those frozen
-- values came from:
--   NULL      -> real per-period edit / historical backfill, or frozen from the anchor row
--   'YYYYMM'  -> frozen copy of that earlier period's snapshot row

ALTER TABLE cst_rm_group_head_period
    ADD COLUMN IF NOT EXISTS carried_from_period VARCHAR(6);

ALTER TABLE cst_rm_group_detail_period
    ADD COLUMN IF NOT EXISTS carried_from_period VARCHAR(6);

COMMENT ON COLUMN cst_rm_group_head_period.carried_from_period IS 'Source period (YYYYMM) this row was frozen from at calc time; NULL = own edit/backfill or frozen from the anchor row.';
COMMENT ON COLUMN cst_rm_group_detail_period.carried_from_period IS 'Source period (YYYYMM) this row was frozen from at calc time; NULL = own edit/backfill or frozen from the anchor row.';

-- Carry-forward lookups: "latest period < $p" per head / per detail.
CREATE INDEX IF NOT EXISTS idx_rm_group_head_period_head_period
    ON cst_rm_group_head_period (group_head_id, period DESC);

CREATE INDEX IF NOT EXISTS idx_rm_group_detail_period_detail_period
    ON cst_rm_group_detail_period (group_detail_id, period DESC);
