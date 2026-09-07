-- Rollback: drop cst_rm_group_head_period and all its indexes.

DROP INDEX IF EXISTS idx_rm_group_head_period_head;
DROP INDEX IF EXISTS uk_rm_group_head_period;

DROP TABLE IF EXISTS cst_rm_group_head_period;
