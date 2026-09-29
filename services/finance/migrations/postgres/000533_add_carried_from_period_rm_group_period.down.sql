-- Rollback: drop carried_from_period and the carry-forward lookup indexes.

DROP INDEX IF EXISTS idx_rm_group_detail_period_detail_period;
DROP INDEX IF EXISTS idx_rm_group_head_period_head_period;

ALTER TABLE cst_rm_group_detail_period DROP COLUMN IF EXISTS carried_from_period;
ALTER TABLE cst_rm_group_head_period DROP COLUMN IF EXISTS carried_from_period;
